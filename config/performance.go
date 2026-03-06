package config

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func PerformanceLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// Process request
		c.Next()
		
		// Calculate latency
		latency := time.Since(start)
		
		// Log all requests with timing
		log.Printf("[PERF] %s %s | Status: %d | Time: %v", 
			c.Request.Method, 
			c.Request.URL.Path, 
			c.Writer.Status(), 
			latency)
		
		// Alert for slow requests
		if latency > 200*time.Millisecond {
			log.Printf("🐌 SLOW REQUEST: %s %s took %v", 
				c.Request.Method, c.Request.URL.Path, latency)
		}
	}
}