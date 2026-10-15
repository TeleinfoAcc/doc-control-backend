package handlers

import (
	"checklist-backend/database"
	"checklist-backend/models"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

// แถวของรายงาน = ข้อมูลคอส + ชื่อเจ้าของคอส
type CourseReportRow struct {
	models.Doccontrol
	Agent_name string `json:"agent_name"`
	Emp_id     string `json:"emp_id"`
}

type ReportAgent struct {
	Agent_id   string `json:"agent_id"`
	Agent_name string `json:"agent_name"`
}

// agent_id ใน transaction เป็น varchar ส่วนใน iam_agents เป็นตัวเลข จึง cast เป็น text ก่อน join
const reportJoin = "LEFT JOIN qamon.iam_agents a ON a.agent_id::text = t.agent_id"
const agentNameExpr = "TRIM(CONCAT_WS(' ', a.title_name_th, a.first_name_th, a.last_name_th))"

// GET /api/reports/courses?year=2026&agent_id=123
// year กรองจาก start_date, ไม่ส่ง = ทุกปี / ไม่ส่ง agent_id = ทุกคน
func GetCourseReport(c *gin.Context) {
	query := database.DB.Table("doccontrol.transaction t").
		Select("t.*, " + agentNameExpr + " AS agent_name, a.emp_id").
		Joins(reportJoin)

	if y := c.Query("year"); y != "" {
		year, err := strconv.Atoi(y)
		if err != nil {
			c.JSON(400, gin.H{"error": "ปีไม่ถูกต้อง"})
			return
		}
		query = query.Where("EXTRACT(YEAR FROM t.start_date) = ?", year)
	}
	if agentID := c.Query("agent_id"); agentID != "" {
		query = query.Where("t.agent_id = ?", agentID)
	}

	var rows []CourseReportRow
	if err := query.Order("agent_name ASC, t.start_date DESC NULLS LAST, t.id DESC").Scan(&rows).Error; err != nil {
		c.JSON(500, gin.H{"error": "ไม่สามารถดึงข้อมูลรายงานได้", "details": err.Error()})
		return
	}

	c.JSON(200, gin.H{"courses": rows})
}

// GET /api/reports/filters → ปีที่มีข้อมูล (จาก start_date) และรายชื่อคนที่มีคอส
func GetReportFilters(c *gin.Context) {
	var years []int
	if err := database.DB.Raw(`SELECT DISTINCT EXTRACT(YEAR FROM start_date)::int AS year
		FROM doccontrol.transaction WHERE start_date IS NOT NULL ORDER BY year DESC`).
		Scan(&years).Error; err != nil {
		c.JSON(500, gin.H{"error": "ไม่สามารถดึงข้อมูลปีได้", "details": err.Error()})
		return
	}

	var agents []ReportAgent
	if err := database.DB.Table("doccontrol.transaction t").
		Select("DISTINCT t.agent_id, " + agentNameExpr + " AS agent_name").
		Joins(reportJoin).
		Order("agent_name ASC").
		Scan(&agents).Error; err != nil {
		c.JSON(500, gin.H{"error": "ไม่สามารถดึงรายชื่อได้", "details": err.Error()})
		return
	}

	c.JSON(200, gin.H{"years": years, "agents": agents})
}

// GET /api/reports/courses/:id/evidence → เปิดไฟล์หลักฐานของคอสใดก็ได้ (ใช้ในหน้ารายงาน)
func GetReportEvidence(c *gin.Context) {
	var course models.Doccontrol
	if err := database.DB.Where("id = ?", c.Param("id")).First(&course).Error; err != nil {
		c.JSON(404, gin.H{"error": "ไม่พบข้อมูลคอส"})
		return
	}
	if course.Evidence_path == "" {
		c.JSON(404, gin.H{"error": "คอสนี้ไม่มีไฟล์หลักฐาน"})
		return
	}
	if _, err := os.Stat(course.Evidence_path); err != nil {
		c.JSON(404, gin.H{"error": "ไม่พบไฟล์หลักฐาน"})
		return
	}

	c.File(course.Evidence_path)
}
