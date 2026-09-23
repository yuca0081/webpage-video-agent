// Package store：元数据持久层（PostgreSQL，GORM 只作查询层不迁移表）。
// PG 不可达时降级内存（仅本进程内聊天记录可用），启动时打警告。
package store

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Msg struct {
	ID        int64  `gorm:"primaryKey"           json:"id"`
	ProjectID string `gorm:"column:project_id"    json:"project_id"` // 磁盘项目 ID（api 主键）
	Role      string `json:"role"`                                  // user | agent | system
	Type      string `json:"type"`                                  // text | tool_call | progress | error
	Content   string `json:"content"`
	ToolName  string `gorm:"column:tool_name"      json:"tool_name,omitempty"`
	ToolArgs  string `gorm:"type:jsonb"            json:"tool_args,omitempty"`
	ToolResult string `gorm:"type:jsonb"           json:"tool_result,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectRow struct {
	ID        string `gorm:"column:disk_id" json:"id"` // 对外一律用磁盘 ID
	UUID      string `gorm:"primaryKey;column:id" json:"-"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ProjectRow) TableName() string { return "projects" }

// Store 聊天案卷 + 项目元数据。chat_messages.project_id 在 DDL 里是 UUID 外键，
// PG 实现按 disk_id 换写；内存实现直接存。
type Store interface {
	CreateProject(diskID, name string) error
	EnsureProject(diskID, name string) error // 已存在则跳过
	UpdateStatus(diskID, status string) error
	ListProjects() ([]ProjectRow, error)
	SaveMsg(m *Msg) error
	ListMsgs(projectID string, afterID int64, limit int) ([]Msg, error)
	Kind() string
}

// Open 先试 PG（3s 超时），失败降级内存。
func Open(dsn string) Store {
	if dsn == "" {
		log.Println("[store] PG_DSN 未配置，用内存存储")
		return newMem()
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn + " connect_timeout=3",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Printf("[store] PG 连接失败（%v），降级内存存储", err)
		return newMem()
	}
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(4)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}
	log.Println("[store] PG 已连接")
	return &pgStore{db: db}
}

type pgStore struct{ db *gorm.DB }

func (s *pgStore) Kind() string { return "pg" }

func (s *pgStore) findUUID(diskID string) (string, error) {
	var row struct {
		UUID string `gorm:"column:id"`
	}
	err := s.db.Table("projects").Select("id").Where("disk_id = ?", diskID).Scan(&row).Error
	if err != nil || row.UUID == "" {
		return "", fmt.Errorf("项目 %s 无 DB 行", diskID)
	}
	return row.UUID, nil
}

func (s *pgStore) CreateProject(diskID, name string) error {
	return s.db.Exec(`INSERT INTO projects (name, status, disk_id) VALUES (?, 'created', ?)`, name, diskID).Error
}

func (s *pgStore) EnsureProject(diskID, name string) error {
	var n int64
	s.db.Table("projects").Where("disk_id = ?", diskID).Count(&n)
	if n > 0 {
		return nil
	}
	return s.CreateProject(diskID, name)
}

func (s *pgStore) UpdateStatus(diskID, status string) error {
	return s.db.Exec(`UPDATE projects SET status = ? WHERE disk_id = ?`, status, diskID).Error
}

func (s *pgStore) ListProjects() ([]ProjectRow, error) {
	var rows []ProjectRow
	err := s.db.Table("projects").
		Select("disk_id, name, status, created_at").
		Where("disk_id IS NOT NULL").
		Order("created_at DESC").Scan(&rows).Error
	return rows, err
}

func (s *pgStore) SaveMsg(m *Msg) error {
	uuid, err := s.findUUID(m.ProjectID)
	if err != nil {
		return err
	}
	return s.db.Exec(`INSERT INTO chat_messages (project_id, role, type, content, tool_name, tool_args, tool_result)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, '')::jsonb, NULLIF(?, '')::jsonb)`,
		uuid, m.Role, m.Type, m.Content, m.ToolName, m.ToolArgs, m.ToolResult).Error
}

func (s *pgStore) ListMsgs(projectID string, afterID int64, limit int) ([]Msg, error) {
	uuid, err := s.findUUID(projectID)
	if err != nil {
		return nil, err
	}
	var rows []Msg
	err = s.db.Table("chat_messages").
		Select("id, role, type, content, tool_name, created_at").
		Where("project_id = ? AND id > ? AND type IN ('text','tool_call','error')", uuid, afterID).
		Order("id ASC").Limit(limit).Scan(&rows).Error
	for i := range rows {
		rows[i].ProjectID = projectID
	}
	return rows, err
}

// ── 内存实现（PG 不可达时的降级）────────────────────────────

type memStore struct {
	mu       sync.Mutex
	projects []ProjectRow
	msgs     []Msg
	seq      int64
}

func newMem() *memStore { return &memStore{} }

func (s *memStore) Kind() string { return "mem" }

func (s *memStore) upsert(diskID, name string) {
	for i := range s.projects {
		if s.projects[i].ID == diskID {
			return
		}
	}
	s.projects = append([]ProjectRow{{ID: diskID, Name: name, Status: "created", CreatedAt: time.Now()}}, s.projects...)
}

func (s *memStore) CreateProject(diskID, name string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.upsert(diskID, name)
	return nil
}

func (s *memStore) EnsureProject(diskID, name string) error { return s.CreateProject(diskID, name) }

func (s *memStore) UpdateStatus(diskID, status string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	for i := range s.projects {
		if s.projects[i].ID == diskID {
			s.projects[i].Status = status
		}
	}
	return nil
}

func (s *memStore) ListProjects() ([]ProjectRow, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	out := make([]ProjectRow, len(s.projects))
	copy(out, s.projects)
	return out, nil
}

func (s *memStore) SaveMsg(m *Msg) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.seq++
	m.ID = s.seq
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	s.msgs = append(s.msgs, *m)
	return nil
}

func (s *memStore) ListMsgs(projectID string, afterID int64, limit int) ([]Msg, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	var out []Msg
	for _, m := range s.msgs {
		if m.ProjectID == projectID && m.ID > afterID && (m.Type == "text" || m.Type == "tool_call" || m.Type == "error") {
			out = append(out, m)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
