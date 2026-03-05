package httpapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

func ResolveFrontendDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(cwd, "frontend"),
		filepath.Join(filepath.Dir(cwd), "frontend"),
		filepath.Join(cwd, "..", "frontend"),
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}
	return filepath.Join(cwd, "frontend"), nil
}

func NewFrontendSidecarHandler(frontendDir, backendURL string, log *zap.Logger) (http.Handler, error) {
	backend, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}

	e := echo.New()
	e.HideBanner = true

	// Serve static frontend files
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:   frontendDir,
		Index:  "index.html",
		Browse: false,
		HTML5:  true,
	}))

	// Proxy API/WS/metrics/health to backend
	e.Group("/*", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return c.Redirect(http.StatusTemporaryRedirect, backend.String()+c.Request().URL.Path)
		}
	})

	return e, nil
}
