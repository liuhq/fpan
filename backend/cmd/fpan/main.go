package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/liuhq/fpan/internal/config"
	"github.com/liuhq/fpan/internal/database"
)

func main() {
	config, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.Open(config.DatabaseUrl)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Migrate(); err != nil {
		log.Fatal(err)
	}

	r := gin.Default()

	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	srv := &http.Server{
		Addr:    config.ListenAddr,
		Handler: r,
	}

	err = srv.ListenAndServe()
	log.Printf("INFO: listening at %s", srv.Addr)
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
