package admin

import (
	"github.com/gin-gonic/gin"
	"goRAGENT/internal/handler/httpx"
)

func (h *Handler) listDocuments(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.Document.ListByKB(c.Request.Context(), c.Param("id"), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) uploadDocument(c *gin.Context) {
	kbID := c.Param("id")
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpx.BadRequest(c, "请选择文件")
		return
	}
	defer file.Close()

	vo, err := h.svc.Document.Upload(c.Request.Context(), kbID, header.Filename, file)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) searchDocuments(c *gin.Context) {
	pq := httpx.PageFromQuery(c)
	vos, total, err := h.svc.Document.Search(c.Request.Context(), c.Query("keyword"), pq)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"total": total, "rows": vos})
}

func (h *Handler) getDocument(c *gin.Context) {
	vo, err := h.svc.Document.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, vo)
}

func (h *Handler) previewDocument(c *gin.Context) {
	content, docName, err := h.svc.Document.Preview(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"content": content, "docName": docName})
}

func (h *Handler) downloadDocument(c *gin.Context) {
	filePath, fileName, err := h.svc.Document.Download(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	c.File(filePath)
}

func (h *Handler) deleteDocument(c *gin.Context) {
	if err := h.svc.Document.Delete(c.Request.Context(), c.Param("id")); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OKEmpty(c)
}

func (h *Handler) toggleDocument(c *gin.Context) {
	enabled, err := h.svc.Document.Toggle(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, gin.H{"enabled": enabled})
}
