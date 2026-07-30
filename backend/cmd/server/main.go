package main

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func initDB() (*sql.DB, error) {
	dbURI := "concession.db" // file-based path for sqlite dev convenience

	db, err := gorm.Open(sqlite.Open(dbURI), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	//err = db.AutoMigrate(&Movie, &Show, &Review)
	err = db.AutoMigrate()
	if err != nil {
		return nil, err
	}
	return nil, err
}

func main() {
	router := gin.Default()
	router.GET("/ping", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	err := router.Run()
	if err != nil {
		return
	}
}
