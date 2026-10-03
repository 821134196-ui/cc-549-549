package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed web/dist
var distFS embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "verify" {
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		dbPath := fs.String("db", "data/verify.db", "SQLite 数据库路径（会重建）")
		_ = fs.Parse(os.Args[2:])
		os.Exit(runVerify(*dbPath))
	}
	if len(os.Args) > 1 && os.Args[1] == "seed" {
		fs := flag.NewFlagSet("seed", flag.ExitOnError)
		dbPath := fs.String("db", "data/royalty.db", "SQLite 数据库路径")
		fresh := fs.Bool("fresh", false, "先删除旧库")
		_ = fs.Parse(os.Args[2:])
		if *fresh {
			os.Remove(*dbPath)
		}
		db, err := openDB(*dbPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := Seed(db); err != nil {
			log.Fatal(err)
		}
		log.Printf("种子数据已写入 %s", *dbPath)
		return
	}

	// 默认 serve
	dbPath := getenv("ROYALTY_DB", "data/royalty.db")
	addr := getenv("ROYALTY_ADDR", ":8080")
	autoSeed := getenv("ROYALTY_SEED", "1") != "0"
	if len(os.Args) > 1 {
		// 兼容 `royalty serve --addr` 形式
		if os.Args[1] == "serve" {
			os.Args = append(os.Args[:1], os.Args[2:]...)
		}
		fl := flag.NewFlagSet("serve", flag.ExitOnError)
		fl.StringVar(&dbPath, "db", dbPath, "")
		fl.StringVar(&addr, "addr", addr, "")
		_ = fl.Parse(os.Args[1:])
	}

	db, err := openDB(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	if autoSeed {
		var n int64
		db.Model(&User{}).Count(&n)
		if n == 0 {
			if err := Seed(db); err != nil {
				log.Fatal(err)
			}
			log.Printf("首次启动：已自动写入示例数据")
		}
	}

	r := Router(db)
	registerStatic(r)
	log.Printf("稿酬结算系统已启动：http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func registerStatic(r *gin.Engine) {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	// 若前端尚未构建（只有占位文件），给出友好提示
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
				`<h2>前端尚未构建</h2><p>请先运行 <code>npm run build</code>（或执行 scripts/run.sh）。API 仍可通过 /api 使用。</p>`))
		})
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		if p != "" {
			if f, err := sub.Open(p); err == nil {
				f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		// SPA 回退
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
