package models

type IamAgents struct {
	Agent_id      uint   `json:"agent_id" gorm:"primaryKey"`
	Title_name_th string `json:"title_name_th"`
	First_name_th string `json:"first_name_th"`
	Last_name_th  string `json:"last_name_th"`
	Title_name_en string `json:"title_name_en"`
	First_name_en string `json:"first_name_en"`
	Last_name_en  string `json:"last_name_en"`
	Displayname   string `json:"displayname"`
	Agent_note    string `json:"agent_note"`
	Role_id       uint   `json:"role_id"`
	Permission_id uint   `json:"permission_id"`
	Position_id   uint   `json:"position_id"`
	Login         string `json:"login"`
	Password      string `json:"-"`
	Tmc_email     string `json:"tmc_email"`
	Contact_no    string `json:"contact_no"`
	Start_date    string `json:"start_date"`
	Emp_id        string `json:"emp_id"`
	Main_proj_id  uint   `json:"main_proj_id"`
	Is_active     uint   `json:"is_active"`
	Hire_date     string `json:"hire_date"`
}

func (IamAgents) TableName() string {
	return "qamon.iam_agents"
}
