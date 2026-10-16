package handlers

import (
	"checklist-backend/database"
	"checklist-backend/models"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// แบบฟอร์ม FM-CCS-TR-001 แบบฟอร์มประวัติการฝึกอบรมรายบุคคล
const (
	trainingTemplatePath = "templates/training_record.xlsx"
	trainingSheet        = "FM-CCS-TR-001"
	trainingFirstRow     = 10 // แถวแรกของตารางประวัติการอบรม
	trainingTemplateRows = 27 // แถวที่มีกรอบเตรียมไว้ใน template (10-36)
)

var thaiMonths = []string{"", "มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน",
	"กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"}

// ข้อมูลพนักงานสำหรับส่วนที่ 1 ของแบบฟอร์ม
type trainingRecordAgent struct {
	Title_name_th string
	First_name_th string
	Last_name_th  string
	Emp_id        string
	Hire_date     *time.Time
	Position_name string
	Proj_name     string
}

// GET /api/reports/agents/:id/training-record?year=2026
// export ประวัติการอบรมของคนเดียวเป็น Excel ตามแบบฟอร์ม FM-CCS-TR-001 (ไม่ส่ง year = ทุกปี)
func ExportTrainingRecord(c *gin.Context) {
	agentID, ok := paramID(c)
	if !ok {
		return
	}

	var agent trainingRecordAgent
	result := database.DB.Raw(`SELECT a.title_name_th, a.first_name_th, a.last_name_th, a.emp_id,
			a.hire_date,
			COALESCE(p.position_name, '') AS position_name, COALESCE(j.proj_name, '') AS proj_name
		FROM qamon.iam_agents a
		LEFT JOIN qamon.iam_position p ON p.position_id = a.position_id
		LEFT JOIN qamon.iam_project j ON j.proj_id = a.main_proj_id
		WHERE a.agent_id = ?`, agentID).Scan(&agent)
	if result.Error != nil {
		internalError(c, "ไม่สามารถดึงข้อมูลพนักงานได้", result.Error)
		return
	}
	// Raw().Scan() ไม่คืน ErrRecordNotFound จึงเช็คจำนวนแถวแทน
	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "ไม่พบข้อมูลพนักงาน"})
		return
	}

	query := database.DB.Where("agent_id = ?", agentID)
	if y := c.Query("year"); y != "" {
		year, err := strconv.Atoi(y)
		if err != nil {
			c.JSON(400, gin.H{"error": "ปีไม่ถูกต้อง"})
			return
		}
		query = query.Where("EXTRACT(YEAR FROM start_date) = ?", year)
	}

	var courses []models.Doccontrol
	if err := query.Order("start_date ASC NULLS LAST, id ASC").Find(&courses).Error; err != nil {
		internalError(c, "ไม่สามารถดึงข้อมูลคอร์สได้", err)
		return
	}

	f, err := buildTrainingRecord(agent, courses)
	if err != nil {
		internalError(c, "ไม่สามารถสร้างไฟล์ Excel ได้", err)
		return
	}
	defer f.Close()

	filename := fmt.Sprintf("TrainingRecord_%s.xlsx", agent.Emp_id)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)))
	if err := f.Write(c.Writer); err != nil {
		internalError(c, "ไม่สามารถส่งไฟล์ Excel ได้", err)
	}
}

// เปิด template แล้วกรอกข้อมูลพนักงาน + ประวัติการอบรม
func buildTrainingRecord(agent trainingRecordAgent, courses []models.Doccontrol) (*excelize.File, error) {
	f, err := excelize.OpenFile(trainingTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("open template: %w", err)
	}

	set := func(cell string, value any) {
		if err == nil {
			err = f.SetCellValue(trainingSheet, cell, value)
		}
	}

	// ส่วนที่ 1: ข้อมูลพนักงาน
	set("D4", strings.TrimSpace(agent.Title_name_th+" "+agent.First_name_th+" "+agent.Last_name_th))
	set("F4", agent.Emp_id)
	set("D5", agent.Position_name)
	set("F5", thaiDate(agent.Hire_date))
	set("D6", "") // แผนก / ฝ่าย: เว้นว่าง (ล้างค่าตัวอย่างใน template)
	set("F6", agent.Proj_name)

	// คอลัมน์ J เป็นคำแนะนำการกรอกฟอร์ม (อยู่นอกพื้นที่พิมพ์) ไม่ต้องติดไปในไฟล์ที่ export
	for row := 3; row <= 9; row++ {
		set(fmt.Sprintf("J%d", row), "")
	}

	// เพิ่มแถวถ้าคอร์สเกินจำนวนแถวที่ template เตรียมไว้ (copy รูปแบบจากแถวสุดท้าย)
	lastRow := trainingFirstRow + trainingTemplateRows - 1
	for extra := len(courses) - trainingTemplateRows; extra > 0; extra-- {
		if err == nil {
			err = f.DuplicateRow(trainingSheet, lastRow)
		}
		lastRow++
	}
	if len(courses) > trainingTemplateRows && err == nil {
		// ขยายพื้นที่พิมพ์ให้ครอบแถวที่เพิ่มมา (ต้องลบชื่อเดิมก่อน ตั้งซ้ำไม่ได้)
		_ = f.DeleteDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", Scope: trainingSheet})
		err = f.SetDefinedName(&excelize.DefinedName{
			Name:     "_xlnm.Print_Area",
			RefersTo: fmt.Sprintf("'%s'!$B$1:$H$%d", trainingSheet, lastRow),
			Scope:    trainingSheet,
		})
	}

	// ส่วนที่ 2: ล้างตัวอย่างใน template แล้วกรอกประวัติการอบรม
	for row := trainingFirstRow; row <= lastRow; row++ {
		set(fmt.Sprintf("B%d", row), row-trainingFirstRow+1)
		for _, col := range []string{"C", "D", "E", "F", "G", "H"} {
			set(fmt.Sprintf("%s%d", col, row), "")
		}
	}
	for i, course := range courses {
		row := trainingFirstRow + i
		trainDate := course.Start_date
		if trainDate == nil {
			trainDate = course.Complete_date
		}
		if trainDate != nil {
			// คอลัมน์วันที่ใน template จัดรูปแบบเป็นวันที่ไทย (พ.ศ.) อยู่แล้ว จึงใส่เป็นค่าวันที่ของ Excel
			set(fmt.Sprintf("C%d", row), excelSerialDate(*trainDate))
		}
		set(fmt.Sprintf("D%d", row), course.Course_name)
		set(fmt.Sprintf("E%d", row), trainingFormat(course.Loc_type))
		set(fmt.Sprintf("H%d", row), trainingNote(course))
	}

	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// รูปแบบการอบรมใน template มีให้เลือก: OJT(On the Job), In-house, Public, On line
func trainingFormat(locType string) string {
	switch locType {
	case "Online":
		return "On line"
	case "Onsite":
		return "In-house"
	default:
		return locType
	}
}

func trainingNote(course models.Doccontrol) string {
	var notes []string
	if course.Course_id != "" {
		notes = append(notes, "รหัส "+course.Course_id)
	}
	switch course.Status {
	case "In Progress":
		notes = append(notes, "กำลังอบรม")
	case "Not Started":
		notes = append(notes, "ยังไม่เริ่มอบรม")
	}
	// คอร์สที่จบแล้วและอบรมหลายวัน ระบุวันที่จบไว้ด้วย (คอลัมน์วันที่ใส่ได้วันเดียว)
	if course.Status == "Completed" && course.Complete_date != nil &&
		(course.Start_date == nil || !sameDay(*course.Start_date, *course.Complete_date)) {
		notes = append(notes, "จบ "+thaiDate(course.Complete_date))
	}
	return strings.Join(notes, ", ")
}

// แปลงวันที่เป็นเลข serial ของ Excel (นับวันจาก 30 ธ.ค. 1899)
func excelSerialDate(t time.Time) float64 {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.Sub(time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)).Hours() / 24
}

// วันที่แบบไทย พ.ศ. เช่น 20 มีนาคม 2569
func thaiDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("%d %s %d", t.Day(), thaiMonths[t.Month()], t.Year()+543)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
