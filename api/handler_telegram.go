package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"nofx/logger"
)

// handleGetTelegramConfig returns current Telegram bot configuration and binding status
func (s *Server) handleGetTelegramConfig(c *gin.Context) {
	cfg, err := s.store.TelegramConfig().Get()
	if err != nil {
		// Not configured yet - return empty state
		c.JSON(http.StatusOK, gin.H{
			"configured":   false,
			"is_bound":     false,
			"token_masked": "",
			"username":     "",
		})
		return
	}

	// Mask bot token for security (show only last 6 chars)
	tokenMasked := ""
	if cfg.BotToken != "" {
		if len(cfg.BotToken) > 6 {
			tokenMasked = "***" + cfg.BotToken[len(cfg.BotToken)-6:]
		} else {
			tokenMasked = "***"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"configured":   cfg.BotToken != "",
		"is_bound":     cfg.ChatID != 0,
		"username":     cfg.Username,
		"bound_at":     cfg.BoundAt,
		"token_masked": tokenMasked,
		"model_id":     cfg.ModelID,
	})
}

// handleUpdateTelegramConfig saves bot token (+ optional model ID) and triggers bot hot-reload
func (s *Server) handleUpdateTelegramConfig(c *gin.Context) {
	var req struct {
		BotToken string `json:"bot_token"`
		ModelID  string `json:"model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	req.BotToken = strings.TrimSpace(req.BotToken)
	if req.BotToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token is required"})
		return
	}

	bot, err := tgbotapi.NewBotAPI(req.BotToken)
	if err != nil {
		logger.Warnf("Telegram token validation failed during save: err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Telegram bot token"})
		return
	}
	if _, err := bot.GetMe(); err != nil {
		logger.Warnf("Telegram getMe failed during save: err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "telegram token failed validation"})
		return
	}

	if err := s.store.TelegramConfig().Save(req.BotToken, req.ModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config"})
		return
	}

	// Signal bot hot-reload if channel is available
	if s.telegramReloadCh != nil {
		select {
		case s.telegramReloadCh <- struct{}{}:
		default: // non-blocking
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Bot token saved. Bot will reload automatically."})
}

// handleUnbindTelegram removes Telegram user binding
func (s *Server) handleUnbindTelegram(c *gin.Context) {
	if err := s.store.TelegramConfig().Unbind(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unbind"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Telegram binding removed"})
}

// handleUpdateTelegramModel updates only the AI model used for Telegram replies (no token re-entry needed)
func (s *Server) handleUpdateTelegramModel(c *gin.Context) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	cfg, err := s.store.TelegramConfig().Get()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Telegram config found, save a bot token first"})
		return
	}

	if err := s.store.TelegramConfig().Save(cfg.BotToken, req.ModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save model config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "model_id": req.ModelID})
}
