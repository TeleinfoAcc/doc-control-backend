package main

import (
	"checklist-backend/cron"
	"checklist-backend/database"
	handlers "checklist-backend/handler"
	"checklist-backend/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cron.StartEmailCron()

	database.Connect()

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:4200", "http://172.21.142.211:8005", "http://10.238.99.152:8005"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))

	// Public routes (ไม่ต้อง login)
	r.POST("/api/auth/login", handlers.Login)

	// Protected routes (ต้อง login)
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	{

		protected.GET("/courses", handlers.GetMyCourses)
		protected.GET("/courses/:id", handlers.GetCourse)
		protected.POST("/courses", handlers.CreateCourse)
		protected.PUT("/courses/:id", handlers.UpdateCourse)
		protected.GET("/courses/:id/evidence", handlers.GetCourseEvidence)

		protected.GET("/reports/courses", handlers.GetCourseReport)
		protected.GET("/reports/courses/:id/evidence", handlers.GetReportEvidence)
		protected.GET("/reports/filters", handlers.GetReportFilters)

		// protected.DELETE("/users/:id", handlers.DeleteUser)
	}

	r.Run(":8080")
}
