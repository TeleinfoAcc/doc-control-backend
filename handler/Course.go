package handlers

import (
	"checklist-backend/database"
	"checklist-backend/models"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	uploadDir       = "uploads/evidence"
	maxEvidenceSize = 10 << 20 // 10MB
)

var allowedEvidenceExt = map[string]bool{
	".pdf": true, ".jpg": true, ".jpeg": true, ".png": true,
}

var unsafeFileChars = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// agent_id ใน JWT claims ถูก decode เป็น float64
// แต่คอลัมน์ agent_id ใน doccontrol.transaction เป็น varchar จึงคืนค่าเป็น string
func currentAgentID(c *gin.Context) (string, bool) {
	v, ok := c.Get("agent_id")
	if !ok {
		return "", false
	}
	f, ok := v.(float64)
	if !ok {
		return "", false
	}
	return strconv.FormatUint(uint64(f), 10), true
}

// หาคอสตาม id ที่เป็นของ agent ที่ login อยู่เท่านั้น
func findOwnCourse(c *gin.Context, agentID string) (*models.Doccontrol, bool) {
	var course models.Doccontrol
	err := database.DB.Where("id = ? AND agent_id = ?", c.Param("id"), agentID).First(&course).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{"error": "ไม่พบข้อมูลคอส"})
		return nil, false
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "เกิดข้อผิดพลาด", "details": err.Error()})
		return nil, false
	}
	return &course, true
}

// อ่านค่าจาก multipart form แล้วใส่ลง course
func bindCourseForm(c *gin.Context, course *models.Doccontrol) error {
	course.Objective = strings.TrimSpace(c.PostForm("objective"))
	course.Course_id = strings.TrimSpace(c.PostForm("course_id"))
	course.Course_name = strings.TrimSpace(c.PostForm("course_name"))
	course.Loc_type = c.PostForm("loc_type")
	course.Subject = strings.TrimSpace(c.PostForm("subject"))
	course.Url = strings.TrimSpace(c.PostForm("url"))
	course.Course_time = strings.TrimSpace(c.PostForm("course_time"))
	course.Status = c.PostForm("status")

	if course.Course_id == "" || course.Course_name == "" {
		return errors.New("กรุณาระบุ ID และ Course Name")
	}

	var err error
	if course.Start_date, err = parseFormDate(c.PostForm("start_date")); err != nil {
		return errors.New("รูปแบบ Start Date ไม่ถูกต้อง")
	}
	if course.Complete_date, err = parseFormDate(c.PostForm("complete_date")); err != nil {
		return errors.New("รูปแบบ Date Complete ไม่ถูกต้อง")
	}
	if course.Start_date != nil && course.Complete_date != nil && course.Complete_date.Before(*course.Start_date) {
		return errors.New("Date Complete ต้องไม่ก่อน Start Date")
	}
	return nil
}

// แปลงค่าจาก input type=date (YYYY-MM-DD) เป็นเวลาไทย ค่าว่างคืน nil
func parseFormDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	t, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// บันทึกไฟล์หลักฐานลง uploads/evidence/<agent_id>/ แล้วคืน path
func saveEvidence(c *gin.Context, agentID string, file *multipart.FileHeader) (string, error) {
	if file.Size > maxEvidenceSize {
		return "", errors.New("ไฟล์มีขนาดเกิน 10MB")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedEvidenceExt[ext] {
		return "", errors.New("รองรับเฉพาะไฟล์ PDF, JPG, PNG")
	}

	dir := filepath.Join(uploadDir, agentID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	base := unsafeFileChars.ReplaceAllString(strings.TrimSuffix(filepath.Base(file.Filename), filepath.Ext(file.Filename)), "_")
	name := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), base, ext)
	path := filepath.ToSlash(filepath.Join(dir, name))

	if err := c.SaveUploadedFile(file, path); err != nil {
		return "", err
	}
	return path, nil
}

func removeEvidence(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func GetMyCourses(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "token ไม่ถูกต้อง"})
		return
	}

	var courses []models.Doccontrol
	if err := database.DB.Where("agent_id = ?", agentID).Order("id desc").Find(&courses).Error; err != nil {
		c.JSON(500, gin.H{"error": "ไม่สามารถดึงข้อมูลคอสได้", "details": err.Error()})
		return
	}

	c.JSON(200, gin.H{"courses": courses})
}

func GetCourse(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "token ไม่ถูกต้อง"})
		return
	}

	course, ok := findOwnCourse(c, agentID)
	if !ok {
		return
	}
	c.JSON(200, gin.H{"course": course})
}

func CreateCourse(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "token ไม่ถูกต้อง"})
		return
	}

	course := models.Doccontrol{Agent_id: agentID}
	if err := bindCourseForm(c, &course); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if file, err := c.FormFile("evidence"); err == nil {
		path, err := saveEvidence(c, agentID, file)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		course.Evidence_path = path
	}

	if err := database.DB.Create(&course).Error; err != nil {
		removeEvidence(course.Evidence_path)
		c.JSON(500, gin.H{"error": "ไม่สามารถเพิ่มคอสได้", "details": err.Error()})
		return
	}

	c.JSON(201, gin.H{"message": "เพิ่มคอสเรียนสำเร็จ", "course": course})
}

func UpdateCourse(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "token ไม่ถูกต้อง"})
		return
	}

	course, ok := findOwnCourse(c, agentID)
	if !ok {
		return
	}
	if err := bindCourseForm(c, course); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	oldEvidence := course.Evidence_path
	if file, err := c.FormFile("evidence"); err == nil {
		path, err := saveEvidence(c, agentID, file)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		course.Evidence_path = path
	} else if c.PostForm("remove_evidence") == "true" {
		course.Evidence_path = ""
	}

	// ใช้ Save เพื่อให้ค่าว่าง / nil (เช่นลบวันที่ หรือลบไฟล์) ถูกบันทึกด้วย
	if err := database.DB.Save(course).Error; err != nil {
		if course.Evidence_path != oldEvidence {
			removeEvidence(course.Evidence_path)
		}
		c.JSON(500, gin.H{"error": "ไม่สามารถอัปเดตคอสได้", "details": err.Error()})
		return
	}

	if course.Evidence_path != oldEvidence {
		removeEvidence(oldEvidence)
	}

	c.JSON(200, gin.H{"message": "อัปเดตคอสเรียนสำเร็จ", "course": course})
}

// ส่งไฟล์หลักฐานให้เฉพาะเจ้าของคอส
func GetCourseEvidence(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "token ไม่ถูกต้อง"})
		return
	}

	course, ok := findOwnCourse(c, agentID)
	if !ok {
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
