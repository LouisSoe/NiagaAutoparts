package scraper

import (
	"context"
	"os"
	"runtime"
)

type PriceResult struct {
    Name        string
    Marketplace string
    Price       float64
    URL         string
}

type Scraper interface {
    Search(ctx context.Context, productName string) ([]PriceResult, error)
}

func defaultChromePath() string {
    switch runtime.GOOS {
    case "windows":
        paths := []string{
            `C:\Program Files\Google\Chrome\Application\chrome.exe`,
            `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
            os.Getenv("LOCALAPPDATA") + `\Google\Chrome\Application\chrome.exe`,
        }
        for _, p := range paths {
            if _, err := os.Stat(p); err == nil {
                return p
            }
        }
    case "darwin":
        return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
    case "linux":
        paths := []string{
            "/usr/bin/google-chrome",
            "/usr/bin/google-chrome-stable",
            "/usr/local/bin/google-chrome",
        }
        for _, p := range paths {
            if _, err := os.Stat(p); err == nil {
                return p
            }
        }
    }
    return ""
}