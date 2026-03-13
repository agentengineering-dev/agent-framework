package main

import (
	"embed"
	"flag"
	"github.com/agentengineering.dev/agent-framework/agent"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"io/fs"
	"log"
	"net/http"
)

// embed all files inside static
//
//go:embed control_plane/*
var staticFiles embed.FS

func main() {

	var envFile = flag.String("env-file", "", "The env file path")

	flag.Parse()
	envFileStr := *envFile

	err := godotenv.Load(envFileStr)
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Create a Gin router with default middleware (logger and recovery)
	r := gin.Default()

	// create sub filesystem
	staticFS, err := fs.Sub(staticFiles, "control_plane")
	if err != nil {
		panic(err)
	}

	r.StaticFS("/ui", http.FS(staticFS))

	// Define a simple GET endpoint
	r.GET("/create-session", func(c *gin.Context) {
		// Return JSON response

		agent.CreateAgentSession(c)

	})

	// Start server on port 8080 (default)
	// Server will listen on 0.0.0.0:8080 (localhost:8080 on Windows)
	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
