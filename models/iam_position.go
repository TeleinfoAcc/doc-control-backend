package models

// field ต้องขึ้นต้นด้วยตัวใหญ่ (exported) ไม่งั้น GORM / JSON มองไม่เห็น
type IamPosition struct {
	PositionId   uint   `json:"position_id" gorm:"column:id;primaryKey"`
	PositionName string `json:"position_name" gorm:"column:position_name"`
	Isactive     string `json:"is_active" gorm:"column:is_active"`
}

func (IamPosition) TableName() string {
	return "qamon.iam_position"
}
