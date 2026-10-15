package handlers

import (
	"checklist-backend/database"
	"checklist-backend/models"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // ฝังข้อมูล timezone ไว้ใน binary เผื่อ image ที่ deploy ไม่มี

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	uploadDir       = "uploads/evidence"
	maxEvidenceSize = 10 << 20 // 10MB
)

// นามสกุลที่รับ → content type ที่เนื้อไฟล์ต้องเป็นจริง (กันการเปลี่ยนนามสกุลหลอก)
var allowedEvidenceTypes = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
}

// \p{M} = สระบน/ล่างและวรรณยุกต์ภาษาไทย (ไม่ใส่จะถูกแทนด้วย _)
var unsafeFileChars = regexp.MustCompile(`[^\p{L}\p{M}\p{N}._-]+`)

var bangkok, _ = time.LoadLocation("Asia/Bangkok")

// ข้อมูลจาก multipart form ของหน้าเพิ่ม/แก้ไขคอส
type courseForm struct {
	Objective       string `form:"objective"`
	Course_id       string `form:"course_id" binding:"required"`
	Course_name     string `form:"course_name" binding:"required"`
	Loc_type        string `form:"loc_type" binding:"required,oneof=Online Onsite"`
	Subject         string `form:"subject"`
	Url             string `form:"url"`
	Course_time     string `form:"course_time"`
	Status          string `form:"status" binding:"required,oneof='Not Started' 'In Progress' Completed"`
	Start_date      string `form:"start_date"`
	Complete_date   string `form:"complete_date"`
	Remove_evidence bool   `form:"remove_evidence"`
}

// agent_id ใน JWT claims ถูก decode เป็น float64
func currentAgentID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("agent_id")
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return uint(f), true
}

// log error ไว้ฝั่ง server แล้วตอบ client แค่ข้อความทั่วไป (ไม่ส่งรายละเอียด DB ออกไป)
func internalError(c *gin.Context, message string, err error) {
	log.Printf("%s %s: %v", c.Request.Method, c.FullPath(), err)
	c.JSON(500, gin.H{"error": message})
}

// อ่าน :id จาก URL ถ้าไม่ใช่ตัวเลขจะตอบ 400
func paramID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "id ไม่ถูกต้อง"})
		return 0, false
	}
	return uint(id), true
}

// หาคอสตาม id ที่เป็นของ agent ที่ login อยู่เท่านั้น
func findOwnCourse(c *gin.Context, agentID uint) (*models.Doccontrol, bool) {
	id, ok := paramID(c)
	if !ok {
		return nil, false
	}

	var course models.Doccontrol
	err := database.DB.Where("id = ? AND agent_id = ?", id, agentID).First(&course).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{"error": "ไม่พบข้อมูลคอส"})
		return nil, false
	}
	if err != nil {
		internalError(c, "เกิดข้อผิดพลาด", err)
		return nil, false
	}
	return &course, true
}

// อ่านค่าจาก multipart form แล้วใส่ลง course
func bindCourseForm(c *gin.Context, course *models.Doccontrol) (*courseForm, error) {
	var form courseForm
	if err := c.ShouldBind(&form); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, errors.New("ไฟล์มีขนาดเกิน 10MB")
		}
		return nil, errors.New("ข้อมูลไม่ครบหรือไม่ถูกต้อง")
	}

	course.Objective = strings.TrimSpace(form.Objective)
	course.Course_id = strings.TrimSpace(form.Course_id)
	course.Course_name = strings.TrimSpace(form.Course_name)
	course.Loc_type = form.Loc_type
	course.Subject = strings.TrimSpace(form.Subject)
	course.Url = strings.TrimSpace(form.Url)
	course.Course_time = strings.TrimSpace(form.Course_time)
	course.Status = form.Status

	if course.Course_id == "" || course.Course_name == "" {
		return nil, errors.New("กรุณาระบุ ID และ Course Name")
	}

	var err error
	if course.Start_date, err = parseFormDate(form.Start_date); err != nil {
		return nil, errors.New("รูปแบบ Start Date ไม่ถูกต้อง")
	}
	if course.Complete_date, err = parseFormDate(form.Complete_date); err != nil {
		return nil, errors.New("รูปแบบ Date Complete ไม่ถูกต้อง")
	}
	if course.Start_date != nil && course.Complete_date != nil && course.Complete_date.Before(*course.Start_date) {
		return nil, errors.New("Date Complete ต้องไม่ก่อน Start Date")
	}
	return &form, nil
}

// แปลงค่าจาก input type=date (YYYY-MM-DD) เป็นเวลาไทย ค่าว่างคืน nil
func parseFormDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", value, bangkok)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// อ่านไฟล์แนบจาก field "evidence" ถ้าไม่ได้แนบคืน nil
func evidenceFile(c *gin.Context) (*multipart.FileHeader, error) {
	file, err := c.FormFile("evidence")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("อ่านไฟล์แนบไม่สำเร็จ")
	}
	return file, nil
}

// ตรวจขนาดและชนิดไฟล์ (จากเนื้อไฟล์จริง) ก่อนบันทึก
func checkEvidence(file *multipart.FileHeader) error {
	if file.Size > maxEvidenceSize {
		return errors.New("ไฟล์มีขนาดเกิน 10MB")
	}
	wantType, ok := allowedEvidenceTypes[strings.ToLower(filepath.Ext(file.Filename))]
	if !ok {
		return errors.New("รองรับเฉพาะไฟล์ PDF, JPG, PNG")
	}

	src, err := file.Open()
	if err != nil {
		return errors.New("อ่านไฟล์แนบไม่สำเร็จ")
	}
	defer src.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(src, head)
	if http.DetectContentType(head[:n]) != wantType {
		return errors.New("รองรับเฉพาะไฟล์ PDF, JPG, PNG")
	}
	return nil
}

// บันทึกไฟล์หลักฐานลง uploads/evidence/<agent_id>/ แล้วคืน path
func saveEvidence(c *gin.Context, agentID uint, file *multipart.FileHeader) (string, error) {
	dir := filepath.Join(uploadDir, fmt.Sprint(agentID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	base := unsafeFileChars.ReplaceAllString(strings.TrimSuffix(filepath.Base(file.Filename), filepath.Ext(file.Filename)), "_")
	name := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), base, ext)
	path := filepath.ToSlash(filepath.Join(dir, name))

	if err := c.SaveUploadedFile(file, path); err != nil {
		return "", err
	}
	return path, nil
}

func removeEvidence(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("remove evidence %q: %v", path, err)
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
		internalError(c, "ไม่สามารถดึงข้อมูลคอสได้", err)
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

	// จำกัดขนาด request ทั้งก้อน กันการส่งไฟล์ใหญ่เกิน
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEvidenceSize+1<<20)

	course := models.Doccontrol{Agent_id: agentID}
	if _, err := bindCourseForm(c, &course); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	file, err := evidenceFile(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if file != nil {
		if err := checkEvidence(file); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		path, err := saveEvidence(c, agentID, file)
		if err != nil {
			internalError(c, "ไม่สามารถบันทึกไฟล์ได้", err)
			return
		}
		course.Evidence_path = path
	}

	if err := database.DB.Create(&course).Error; err != nil {
		removeEvidence(course.Evidence_path)
		internalError(c, "ไม่สามารถเพิ่มคอสได้", err)
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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEvidenceSize+1<<20)

	course, ok := findOwnCourse(c, agentID)
	if !ok {
		return
	}
	form, err := bindCourseForm(c, course)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	file, err := evidenceFile(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	oldEvidence := course.Evidence_path
	if file != nil {
		if err := checkEvidence(file); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		path, err := saveEvidence(c, agentID, file)
		if err != nil {
			internalError(c, "ไม่สามารถบันทึกไฟล์ได้", err)
			return
		}
		course.Evidence_path = path
	} else if form.Remove_evidence {
		course.Evidence_path = ""
	}

	// ใช้ Save เพื่อให้ค่าว่าง / nil (เช่นลบวันที่ หรือลบไฟล์) ถูกบันทึกด้วย
	if err := database.DB.Save(course).Error; err != nil {
		if course.Evidence_path != oldEvidence {
			removeEvidence(course.Evidence_path)
		}
		internalError(c, "ไม่สามารถอัปเดตคอสได้", err)
		return
	}

	// ลบไฟล์เดิมหลัง DB บันทึกสำเร็จแล้วเท่านั้น
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
	sendEvidence(c, course)
}

// ส่งไฟล์หลักฐานของคอสกลับไป (ใช้ทั้งหน้าคอสของฉันและหน้ารายงาน)
func sendEvidence(c *gin.Context, course *models.Doccontrol) {
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
