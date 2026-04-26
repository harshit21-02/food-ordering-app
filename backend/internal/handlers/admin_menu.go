package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/models"
)

type AdminMenuHandler struct {
	DB *gorm.DB
}

func NewAdminMenuHandler(db *gorm.DB) *AdminMenuHandler {
	return &AdminMenuHandler{DB: db}
}

// GET /admin/menu — list all items in the org's menu (incl. unavailable).
func (h *AdminMenuHandler) List(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	var items []models.MenuItem
	if err := h.DB.
		Where("org_id = ?", orgID).
		Order("category NULLS LAST, display_order, name").
		Find(&items).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type createMenuBody struct {
	Name         string  `json:"name"          binding:"required"`
	Description  *string `json:"description"`
	Category     *string `json:"category"`
	Price        float64 `json:"price"         binding:"required"`
	ImageURL     *string `json:"image_url"`
	DisplayOrder int     `json:"display_order"`
	IsAvailable  *bool   `json:"is_available"`
}

// POST /admin/menu (admin-only)
func (h *AdminMenuHandler) Create(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	var body createMenuBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if body.Price <= 0 {
		RespondError(c, http.StatusBadRequest, "invalid_price", "price must be > 0")
		return
	}
	available := true
	if body.IsAvailable != nil {
		available = *body.IsAvailable
	}
	row := models.MenuItem{
		OrgID:        orgID,
		Name:         strings.TrimSpace(body.Name),
		Description:  trimPtr(body.Description),
		Category:     trimPtr(body.Category),
		Price:        body.Price,
		ImageURL:     trimPtr(body.ImageURL),
		DisplayOrder: body.DisplayOrder,
		IsAvailable:  available,
	}
	if err := h.DB.Create(&row).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, row)
}

type updateMenuBody struct {
	Name         *string  `json:"name"`
	Description  *string  `json:"description"`
	Category     *string  `json:"category"`
	Price        *float64 `json:"price"`
	ImageURL     *string  `json:"image_url"`
	DisplayOrder *int     `json:"display_order"`
	IsAvailable  *bool    `json:"is_available"`
}

// PATCH /admin/menu/:id (admin-only)
func (h *AdminMenuHandler) Update(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	var body updateMenuBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	var row models.MenuItem
	if err := h.DB.Where("id = ? AND org_id = ?", id, orgID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "not_found", "menu item not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	updates := map[string]any{}
	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Description != nil {
		updates["description"] = trimPtr(body.Description)
	}
	if body.Category != nil {
		updates["category"] = trimPtr(body.Category)
	}
	if body.Price != nil {
		if *body.Price <= 0 {
			RespondError(c, http.StatusBadRequest, "invalid_price", "price must be > 0")
			return
		}
		updates["price"] = *body.Price
	}
	if body.ImageURL != nil {
		// "" → NULL; otherwise set value.
		updates["image_url"] = trimPtr(body.ImageURL)
	}
	if body.DisplayOrder != nil {
		updates["display_order"] = *body.DisplayOrder
	}
	if body.IsAvailable != nil {
		updates["is_available"] = *body.IsAvailable
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, row)
		return
	}
	if err := h.DB.Model(&row).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	h.DB.First(&row, id)
	c.JSON(http.StatusOK, row)
}

// POST /admin/menu/:id/image  (multipart, field "file")
// Saves the file under ./uploads/menu/<id>-<rand>.<ext> and updates image_url
// on the menu row.
//
// TODO(cloudinary): swap local filesystem for Cloudinary or S3.
func (h *AdminMenuHandler) UploadImage(c *gin.Context) {
	orgID := c.GetInt64("org_id")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	var row models.MenuItem
	if err := h.DB.Where("id = ? AND org_id = ?", id, orgID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			RespondError(c, http.StatusNotFound, "not_found", "menu item not found")
			return
		}
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		RespondError(c, http.StatusBadRequest, "no_file", "expected a multipart 'file' field")
		return
	}
	if fh.Size > 5<<20 { // 5 MB cap
		RespondError(c, http.StatusBadRequest, "file_too_large", "max image size is 5 MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
	default:
		RespondError(c, http.StatusBadRequest, "bad_extension", "image must be jpg/jpeg/png/webp/gif")
		return
	}

	if err := os.MkdirAll("uploads/menu", 0o755); err != nil {
		RespondError(c, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}
	var rand4 [4]byte
	_, _ = rand.Read(rand4[:])
	filename := fmt.Sprintf("%d-%s%s", row.ID, hex.EncodeToString(rand4[:]), ext)
	dest := filepath.Join("uploads", "menu", filename)

	in, err := fh.Open()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		RespondError(c, http.StatusInternalServerError, "fs_error", err.Error())
		return
	}

	url := "/uploads/menu/" + filename
	if err := h.DB.Model(&row).UpdateColumn("image_url", url).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	row.ImageURL = &url
	c.JSON(http.StatusOK, row)
}

// trimPtr returns a *string with the trimmed value, or nil if the trimmed
// value is empty. Lets clients clear nullable fields by sending "".
func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
