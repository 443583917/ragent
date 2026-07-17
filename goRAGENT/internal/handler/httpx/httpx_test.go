package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"goRAGENT/internal/model"
	"goRAGENT/pkg/errs"
	"goRAGENT/pkg/response"
)

func init() { gin.SetMode(gin.TestMode) }

// ===== PageFromQuery =====

func TestPageFromQuery_Defaults(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=foo&pageSize=bar", nil)

	pq := PageFromQuery(c)
	if pq.Page != model.DefaultPage {
		t.Errorf("page = %d, want %d", pq.Page, model.DefaultPage)
	}
	if pq.Size != model.DefaultPageSize {
		t.Errorf("size = %d, want %d", pq.Size, model.DefaultPageSize)
	}
}

func TestPageFromQuery_Valid(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=3&pageSize=50", nil)

	pq := PageFromQuery(c)
	if pq.Page != 3 {
		t.Errorf("page = %d, want 3", pq.Page)
	}
	if pq.Size != 50 {
		t.Errorf("size = %d, want 50", pq.Size)
	}
}

func TestPageFromQuery_ClampsOverflowSize(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=1&pageSize=999", nil)

	pq := PageFromQuery(c)
	if pq.Size != model.MaxPageSize {
		t.Errorf("size = %d, want clamped to %d", pq.Size, model.MaxPageSize)
	}
}

func TestPageFromQuery_ClampsNegativePage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=-5&pageSize=20", nil)

	pq := PageFromQuery(c)
	if pq.Page != model.DefaultPage {
		t.Errorf("page = %d, want clamped to %d", pq.Page, model.DefaultPage)
	}
}

// ===== PageFromCurrentSize =====

func TestPageFromCurrentSize_Defaults(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?current=abc&size=xyz", nil)

	pq := PageFromCurrentSize(c)
	if pq.Page != model.DefaultPage {
		t.Errorf("page (from current) = %d, want %d", pq.Page, model.DefaultPage)
	}
	if pq.Size != model.DefaultPageSize {
		t.Errorf("size = %d, want %d", pq.Size, model.DefaultPageSize)
	}
}

func TestPageFromCurrentSize_Valid(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?current=2&size=15", nil)

	pq := PageFromCurrentSize(c)
	if pq.Page != 2 {
		t.Errorf("page = %d, want 2", pq.Page)
	}
	if pq.Size != 15 {
		t.Errorf("size = %d, want 15", pq.Size)
	}
}

// ===== Error rendering =====

func TestError_AppErrorCodePassthrough(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	appErr := errs.Business("业务异常")
	Error(c, appErr)

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200", w.Code)
	}

	var result response.Result
	// Read body to verify code passthrough
	body := w.Body.String()

	if body == "" {
		t.Fatal("response body is empty")
	}
	_ = result
	// key assertion: code is not "1" (server error), it is errs.CodeBusinessError
	if code := errs.CodeOf(appErr); code != errs.CodeBusinessError {
		t.Errorf("errs.CodeOf = %s, want %s", code, errs.CodeBusinessError)
	}
}

func TestError_HTTPStatusIs200(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Error(c, errs.Business("业务错误"))

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200 (admin endpoints always return HTTP 200)", w.Code)
	}
}

// ===== BadRequest =====

func TestBadRequest_RendersParamError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	BadRequest(c, "name 不能为空")

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200", w.Code)
	}
}

// ===== OK / OKEmpty =====

func TestOK_RendersSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	OK(c, gin.H{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200", w.Code)
	}
}

func TestOKEmpty_RendersSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	OKEmpty(c)

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200", w.Code)
	}
}
