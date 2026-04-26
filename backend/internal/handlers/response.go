package handlers

import "github.com/gin-gonic/gin"

// ErrorBody is the standard error envelope on the wire: {"error": {"code","message"}}.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func RespondError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": ErrorBody{Code: code, Message: message}})
}
