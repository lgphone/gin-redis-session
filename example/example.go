package main

import (
	"github.com/gin-gonic/gin"
	"github.com/lgphone/gin-redis-session/v1"
)

func main() {
	option := &session.Option{
		Host:       "127.0.0.1:6379",
		Password:   "abc123",
		DB:         16,
		MaxActive:  100,
		KeyPrefix:  "c_session:",
		MaxAge:     3600,
		CookieName: "session",
		Domain:     "127.0.0.1",
		Path:       "/",
		HttpOnly:   true,
	}
	r := gin.Default()
	r.Use(session.Middleware(session.Init(option)))

	r.GET("/hello", func(c *gin.Context) {
		c.JSON(200, gin.H{"hello": "world"})
	})
	r.GET("/s", func(c *gin.Context) {
		s := session.GetSession(c)
		val := s.Get("name")
		newVal := c.Query("name")
		s.Set("name", newVal)
		if err := s.Save(); err != nil {
			c.JSON(500, gin.H{"err": err.Error()})
			c.Abort()
			return
		}
		c.JSON(200, gin.H{"s": val})
	})
	r.GET("/out", func(c *gin.Context) {
		s := session.GetSession(c)
		s.Clear()
		if err := s.Save(); err != nil {
			c.JSON(500, gin.H{"err": err.Error()})
			c.Abort()
			return
		}
		c.JSON(200, gin.H{"s": "success"})
	})
	r.Run(":8000")
}
