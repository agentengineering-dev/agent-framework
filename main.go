package main

import (
	"flag"
	"github.com/agentengineering.dev/agent-framework/agent"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"log"
)

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

	// Define a simple GET endpoint
	r.POST("/create-session", func(c *gin.Context) {
		// Return JSON response

		agent.CreateAgentSession(c)

	})

	// Start server on port 8080 (default)
	// Server will listen on 0.0.0.0:8080 (localhost:8080 on Windows)
	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
