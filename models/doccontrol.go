package models

import "time"

// field ต้องขึ้นต้นด้วยตัวใหญ่ (exported) ไม่งั้น GORM / JSON มองไม่เห็น
type Doccontrol struct {
	Id            uint       `json:"id" gorm:"primaryKey"`
	Objective     string     `json:"objective" gorm:"column:objective"`
	Course_id     string     `json:"course_id" gorm:"column:course_id"`
	Course_name   string     `json:"course_name" gorm:"column:course_name"`
	Loc_type      string     `json:"loc_type" gorm:"column:loc_type"`
	Subject       string     `json:"subject" gorm:"column:subject"`
	Url           string     `json:"url" gorm:"column:url"`
	Agent_id      string     `json:"agent_id" gorm:"column:agent_id"`
	Course_time   string     `json:"course_time" gorm:"column:course_time"`
	Status        string     `json:"status" gorm:"column:status"`
	Start_date    *time.Time `json:"start_date" gorm:"column:start_date"`
	Complete_date *time.Time `json:"complete_date" gorm:"column:complete_date"`
	Evidence_path string     `json:"evidence_path" gorm:"column:evidence_path"`
}

func (Doccontrol) TableName() string {
	return "doccontrol.transaction"
}
