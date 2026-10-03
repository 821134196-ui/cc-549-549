package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Server struct {
	db   *gorm.DB
	mu   sync.Mutex // 结算重算串行化
	auth *tokenStore
}

type tokenStore struct {
	mu     sync.RWMutex
	tokens map[string]User
}

func newTokenStore() *tokenStore { return &tokenStore{tokens: map[string]User{}} }

func (ts *tokenStore) issue(u User) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	t := hex.EncodeToString(b)
	ts.mu.Lock()
	ts.tokens[t] = u
	ts.mu.Unlock()
	return t
}

func (ts *tokenStore) get(t string) (User, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	u, ok := ts.tokens[t]
	return u, ok
}

func openDB(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		}),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&User{}, &Author{}, &Work{}, &ContractVersion{}, &ContractParticipant{},
		&ContractRegion{}, &ContractLanguage{}, &SalesBatch{}, &SalesRecord{},
		&Settlement{}, &SettlementLine{}, &Adjustment{}, &PaymentReceipt{},
	); err != nil {
		return nil, err
	}
	return db, nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "seed":
		runSeed(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	case "reset":
		runReset(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Println(`用法: royalty <serve|seed|verify|reset> [参数]
  serve   --addr :8080 --db data/royalty.db --web web   启动服务（空库自动播种并导入示例销量）
  seed    --db data/royalty.db                           写入示例主数据与示例销量文件
  verify  [--keep]                                       用临时库跑全部业务规则断言
  reset   --db data/royalty.db                           清空重建数据库`)
	os.Exit(1)
}

func dbFlag(fs *flag.FlagSet) *string {
	return fs.String("db", "data/royalty.db", "SQLite 数据库路径")
}

func runSeed(args []string) {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	dbPath := dbFlag(fs)
	_ = fs.Parse(args)
	db, err := openDB(*dbPath)
	must(err)
	must(SeedMasterData(db))
	fmt.Println("✓ 主数据已写入", *dbPath)
}

func runReset(args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	dbPath := dbFlag(fs)
	_ = fs.Parse(args)
	_ = os.Remove(*dbPath)
	db, err := openDB(*dbPath)
	must(err)
	must(SeedMasterData(db))
	for _, f := range sampleFiles {
		fmt.Println("✓ 示例文件", f)
	}
	fmt.Println("✓ 数据库已重建并播种", *dbPath)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "监听地址")
	dbPath := dbFlag(fs)
	webDir := fs.String("web", "web", "前端静态目录")
	_ = fs.Parse(args)

	db, err := openDB(*dbPath)
	must(err)

	var userCount int64
	db.Model(&User{}).Count(&userCount)
	if userCount == 0 {
		must(SeedMasterData(db))
		fmt.Println("✓ 空库已自动播种示例数据")
	}
	// 自动导入尚未导入的示例销量文件（内容哈希幂等，重复启动安全）
	imported := 0
	for _, name := range sampleFiles {
		path := filepath.Join("data", name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		res, err := ImportSales(db, name, f)
		f.Close()
		if err != nil {
			fmt.Printf("· %s: %v\n", name, err)
			continue
		}
		fmt.Printf("✓ 导入 %s: %d 行\n", name, res.ImportedRows)
		imported += res.ImportedRows
	}
	if imported > 0 {
		if recs, lines, err := RegenerateSettlements(db); err == nil {
			fmt.Printf("✓ 已生成结算：扫描 %d 条销量，产生 %d 条作者结算明细\n", recs, lines)
		}
	}

	s := &Server{db: db, auth: newTokenStore()}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	s.registerRoutes(r, *webDir)

	fmt.Println("稿酬结算系统已启动: http://localhost" + *addr)
	fmt.Println("演示账号:  editor/editor123 (编辑)  finance/finance123 (财务)  wangming/author123  lihua/author123 (作者)")
	must(r.Run(*addr))
}

func (s *Server) registerRoutes(r *gin.Engine, webDir string) {
	api := r.Group("/api")
	api.POST("/login", s.login)

	authed := api.Group("")
	authed.Use(s.authMiddleware())
	{
		authed.GET("/me", s.me)
		authed.GET("/authors", s.listAuthors)
		authed.GET("/works", s.listWorks)
		authed.GET("/works/:id/authorization", s.workAuthorization)
		authed.GET("/works/:id/contracts", s.listContracts)

		// 编辑：作品 / 作者 / 合同维护
		editor := authed.Group("")
		editor.Use(s.requireRole("editor"))
		editor.POST("/authors", s.createAuthor)
		editor.POST("/works", s.createWork)
		editor.POST("/works/:id/contracts", s.createContract)

		// 财务：销量与结算
		finance := authed.Group("")
		finance.Use(s.requireRole("finance"))
		finance.POST("/sales/import", s.importSales)
		finance.GET("/batches", s.listBatches)
		finance.POST("/settlements/regenerate", s.regenerate)
		finance.POST("/settlements/:id/confirm", s.confirmSettlement)
		finance.POST("/settlements/:id/adjust", s.createAdjustment)
		finance.POST("/settlements/:id/pay", s.paySettlement)

		// 财务与编辑可浏览全部结算；作者仅本人
		authed.GET("/settlements", s.listSettlements)
		authed.GET("/settlements/:id", s.getSettlement)
		authed.GET("/sales/exceptions", s.listExceptions)
	}

	r.Static("/static", webDir)
	r.StaticFile("/vendor/vue.js", filepath.Join(webDir, "vendor", "vue.global.prod.js"))
	r.GET("/", func(c *gin.Context) { c.File(filepath.Join(webDir, "index.html")) })
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		c.File(filepath.Join(webDir, "index.html"))
	})
}

func (s *Server) login(c *gin.Context) {
	var req struct {
		LoginName string `json:"login_name"`
		Password  string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求格式错误"})
		return
	}
	var u User
	if err := s.db.Where("login_name = ? AND password = ?", req.LoginName, req.Password).First(&u).Error; err != nil {
		c.JSON(401, gin.H{"error": "账号或密码错误"})
		return
	}
	token := s.auth.issue(u)
	c.JSON(200, gin.H{"token": token, "user": u})
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		t := strings.TrimPrefix(h, "Bearer ")
		u, ok := s.auth.get(t)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "未登录或登录已失效"})
			return
		}
		c.Set("user", u)
		c.Next()
	}
}

func (s *Server) requireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.MustGet("user").(User)
		for _, r := range roles {
			if u.Role == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"error": "当前角色无权执行该操作"})
	}
}

func currentUser(c *gin.Context) User { return c.MustGet("user").(User) }

func (s *Server) me(c *gin.Context) { c.JSON(200, gin.H{"user": currentUser(c)}) }

func (s *Server) listAuthors(c *gin.Context) {
	var authors []Author
	s.db.Order("id").Find(&authors)
	c.JSON(200, authors)
}

func (s *Server) createAuthor(c *gin.Context) {
	var req struct{ Code, Name string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" || req.Name == "" {
		c.JSON(400, gin.H{"error": "编号与姓名必填"})
		return
	}
	a := Author{Code: req.Code, Name: req.Name}
	if err := s.db.Create(&a).Error; err != nil {
		c.JSON(409, gin.H{"error": "作者编号已存在"})
		return
	}
	c.JSON(200, a)
}

func (s *Server) listWorks(c *gin.Context) {
	var works []Work
	s.db.Order("id").Find(&works)
	c.JSON(200, works)
}

func (s *Server) createWork(c *gin.Context) {
	var req struct{ Code, Title, ISBN string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" || req.Title == "" {
		c.JSON(400, gin.H{"error": "编号与书名必填"})
		return
	}
	w := Work{Code: req.Code, Title: req.Title, ISBN: req.ISBN}
	if err := s.db.Create(&w).Error; err != nil {
		c.JSON(409, gin.H{"error": "作品编号已存在"})
		return
	}
	c.JSON(200, w)
}

func (s *Server) listContracts(c *gin.Context) {
	id := c.Param("id")
	var versions []ContractVersion
	if err := s.db.Where("work_id = ?", id).Order("effective_from").Find(&versions).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	for i := range versions {
		if err := loadContractDetail(s.db, &versions[i]); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(200, versions)
}

// workAuthorization 作品页直接查询授权范围（可指定日期，默认今天）
func (s *Server) workAuthorization(c *gin.Context) {
	id := c.Param("id")
	var w Work
	if err := s.db.First(&w, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "作品不存在"})
		return
	}
	date := time.Now()
	if d := c.Query("date"); d != "" {
		if t, err := time.ParseInLocation("2006-01-02", d, time.Local); err == nil {
			date = t
		}
	}
	var versions []ContractVersion
	s.db.Where("work_id = ?", id).Order("effective_from").Find(&versions)
	for i := range versions {
		_ = loadContractDetail(s.db, &versions[i])
	}
	effective, _ := effectiveContract(s.db, w.ID, date)
	c.JSON(200, gin.H{"work": w, "query_date": date.Format("2006-01-02"), "effective": effective, "versions": versions})
}

type contractReq struct {
	EffectiveFrom  string `json:"effective_from"`
	EffectiveUntil string `json:"effective_until"`
	PrintRateBPS   int    `json:"print_rate_bps"`
	EbookRateBPS   int    `json:"ebook_rate_bps"`
	Note           string `json:"note"`
	Participants   []struct {
		AuthorID uint `json:"author_id"`
		ShareBPS int  `json:"share_bps"`
	} `json:"participants"`
	Regions   []string `json:"regions"`
	Languages []string `json:"languages"`
}

func (s *Server) createContract(c *gin.Context) {
	workID := c.Param("id")
	var w Work
	if err := s.db.First(&w, workID).Error; err != nil {
		c.JSON(404, gin.H{"error": "作品不存在"})
		return
	}
	var req contractReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求格式错误"})
		return
	}
	from, err := time.ParseInLocation("2006-01-02", req.EffectiveFrom, time.Local)
	if err != nil {
		c.JSON(400, gin.H{"error": "生效日期格式应为 YYYY-MM-DD"})
		return
	}
	if len(req.Participants) == 0 {
		c.JSON(400, gin.H{"error": "至少选择一位作者"})
		return
	}
	shareSum := 0
	seen := map[uint]bool{}
	for _, p := range req.Participants {
		if p.ShareBPS <= 0 || p.ShareBPS > 10000 || seen[p.AuthorID] {
			c.JSON(400, gin.H{"error": "分成比例必须为正且作者不重复"})
			return
		}
		seen[p.AuthorID] = true
		shareSum += p.ShareBPS
	}
	if shareSum != 10000 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("作者分成合计须为 100%%，当前 %d 基点（%.2f%%）", shareSum, float64(shareSum)/100)})
		return
	}
	if len(req.Regions) == 0 || len(req.Languages) == 0 {
		c.JSON(400, gin.H{"error": "授权区域与语种不能为空"})
		return
	}
	if req.PrintRateBPS < 0 || req.PrintRateBPS > 10000 || req.EbookRateBPS < 0 || req.EbookRateBPS > 10000 {
		c.JSON(400, gin.H{"error": "费率应在 0~100% 之间"})
		return
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		var maxNo int
		tx.Model(&ContractVersion{}).Where("work_id = ?", w.ID).Select("COALESCE(MAX(version_no),0)").Scan(&maxNo)
		v := ContractVersion{
			WorkID:        w.ID,
			VersionNo:     maxNo + 1,
			EffectiveFrom: from,
			PrintRateBPS:  req.PrintRateBPS,
			EbookRateBPS:  req.EbookRateBPS,
			Note:          req.Note,
		}
		if req.EffectiveUntil != "" {
			until, err := time.ParseInLocation("2006-01-02", req.EffectiveUntil, time.Local)
			if err != nil {
				return fmt.Errorf("失效日期格式应为 YYYY-MM-DD")
			}
			v.EffectiveUntil = &until
		}
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		for _, p := range req.Participants {
			if err := tx.Create(&ContractParticipant{ContractVersionID: v.ID, AuthorID: p.AuthorID, ShareBPS: p.ShareBPS}).Error; err != nil {
				return err
			}
		}
		for _, rg := range req.Regions {
			if err := tx.Create(&ContractRegion{ContractVersionID: v.ID, Region: rg}).Error; err != nil {
				return err
			}
		}
		for _, lg := range req.Languages {
			if err := tx.Create(&ContractLanguage{ContractVersionID: v.ID, Language: lg}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) importSales(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "请上传 CSV 文件（字段 file）"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := ImportSales(s.db, fh.Filename, f)
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	if recs, lines, err := RegenerateSettlements(s.db); err == nil {
		res.Errors = append(res.Errors, fmt.Sprintf("已重算：扫描 %d 条销量，产生 %d 条作者结算明细", recs, lines))
	}
	c.JSON(200, res)
}

func (s *Server) listBatches(c *gin.Context) {
	var batches []SalesBatch
	s.db.Order("id DESC").Find(&batches)
	c.JSON(200, batches)
}

func (s *Server) regenerate(c *gin.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, lines, err := RegenerateSettlements(s.db)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "scanned_records": recs, "generated_lines": lines})
}

type settlementDetail struct {
	Settlement
	Author       Author              `json:"author"`
	Lines        []SettlementLine    `json:"lines"`
	SalesRefs    map[uint]SalesTrace `json:"sales_refs"`
	Adjustments  []Adjustment        `json:"adjustments"`
	Receipt      *PaymentReceipt     `json:"receipt"`
	Linked       *Settlement         `json:"linked_settlement"`
	PayableCents int64               `json:"payable_cents"`
	WorkTitles   map[uint]string     `json:"work_titles"`
}

// SalesTrace 结算行指向的原始销量证据
type SalesTrace struct {
	Ref            string `json:"ref"`
	BatchID        uint   `json:"batch_id"`
	Qty            int64  `json:"qty"`
	Period         string `json:"period"`
	OriginalRef    string `json:"original_ref"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

func (s *Server) scopeSettlements(q *gorm.DB, u User) *gorm.DB {
	if u.Role == "author" {
		if u.AuthorID == nil {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("author_id = ?", *u.AuthorID)
		}
	}
	return q
}

func (s *Server) listSettlements(c *gin.Context) {
	u := currentUser(c)
	q := s.db.Model(&Settlement{})
	q = s.scopeSettlements(q, u)
	if period := c.Query("period"); period != "" {
		q = q.Where("period = ?", period)
	}
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	var list []Settlement
	q.Order("period DESC, author_id, kind").Find(&list)

	authorIDs := map[uint]bool{}
	for _, st := range list {
		authorIDs[st.AuthorID] = true
	}
	names := map[uint]string{}
	if len(authorIDs) > 0 {
		var authors []Author
		s.db.Find(&authors)
		for _, a := range authors {
			names[a.ID] = a.Name
		}
	}
	c.JSON(200, gin.H{"settlements": list, "author_names": names})
}

func (s *Server) getSettlement(c *gin.Context) {
	u := currentUser(c)
	id := c.Param("id")
	var st Settlement
	if err := s.db.First(&st, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "结算单不存在"})
		return
	}
	if u.Role == "author" && (u.AuthorID == nil || *u.AuthorID != st.AuthorID) {
		c.JSON(403, gin.H{"error": "只能查看本人的结算单"})
		return
	}
	var author Author
	s.db.First(&author, st.AuthorID)
	var lines []SettlementLine
	s.db.Where("settlement_id = ?", st.ID).Order("category, id").Find(&lines)
	var adjustments []Adjustment
	s.db.Where("settlement_id = ?", st.ID).Order("id").Find(&adjustments)
	var receipt *PaymentReceipt
	var r0 PaymentReceipt
	if err := s.db.Where("settlement_id = ?", st.ID).First(&r0).Error; err == nil {
		receipt = &r0
	}
	var linked *Settlement
	if st.LinkedSettlementID != nil {
		var l Settlement
		if err := s.db.First(&l, *st.LinkedSettlementID).Error; err == nil {
			linked = &l
		}
	}
	workIDs := map[uint]bool{}
	for _, ln := range lines {
		workIDs[ln.WorkID] = true
	}
	titles := map[uint]string{}
	for wid := range workIDs {
		var w Work
		if s.db.First(&w, wid).Error == nil {
			titles[wid] = w.Title
		}
	}
	// 追溯每行对应的原始销量（渠道行号、批次、数量、退货抵扣依据）
	traces := map[uint]SalesTrace{}
	for _, ln := range lines {
		if _, ok := traces[ln.SalesRecordID]; ok {
			continue
		}
		var sr SalesRecord
		if s.db.First(&sr, ln.SalesRecordID).Error == nil {
			traces[ln.SalesRecordID] = SalesTrace{
				Ref: sr.Ref, BatchID: sr.BatchID, Qty: sr.Qty, Period: sr.Period,
				OriginalRef: sr.OriginalRef, UnitPriceCents: sr.UnitPriceCents,
			}
		}
	}
	payable := st.NormalAmountCents
	for _, a := range adjustments {
		payable += a.AmountCents
	}
	c.JSON(200, settlementDetail{
		Settlement: st, Author: author, Lines: lines, SalesRefs: traces,
		Adjustments: adjustments, Receipt: receipt, Linked: linked,
		PayableCents: payable, WorkTitles: titles,
	})
}

func (s *Server) confirmSettlement(c *gin.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := c.Param("id")
	var st Settlement
	if err := s.db.First(&st, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "结算单不存在"})
		return
	}
	if st.Status != StatusDraft {
		c.JSON(409, gin.H{"error": "只有草稿结算单可以确认；已确认单不得修改金额，请开补差单"})
		return
	}
	now := time.Now()
	st.Status = StatusConfirmed
	st.ConfirmedAt = &now
	if err := s.db.Save(&st).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) createAdjustment(c *gin.Context) {
	var req struct {
		AmountCents int64  `json:"amount_cents"`
		Reason      string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		c.JSON(400, gin.H{"error": "补差金额与原因必填"})
		return
	}
	id := c.Param("id")
	var st Settlement
	if err := s.db.First(&st, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "结算单不存在"})
		return
	}
	if st.Status == StatusDraft {
		c.JSON(409, gin.H{"error": "草稿单可直接重算，无需补差；补差单仅用于已确认/已付款结算单"})
		return
	}
	a := Adjustment{SettlementID: st.ID, AuthorID: st.AuthorID, AmountCents: req.AmountCents, Reason: req.Reason}
	if err := s.db.Create(&a).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "adjustment": a})
}

func (s *Server) paySettlement(c *gin.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := c.Param("id")
	var st Settlement
	if err := s.db.First(&st, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "结算单不存在"})
		return
	}
	if st.Status != StatusConfirmed {
		c.JSON(409, gin.H{"error": "只有已确认结算单可以付款"})
		return
	}
	var adjustments []Adjustment
	s.db.Where("settlement_id = ?", st.ID).Find(&adjustments)
	payable := st.NormalAmountCents
	for _, a := range adjustments {
		payable += a.AmountCents
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	receipt := PaymentReceipt{
		SettlementID: st.ID,
		AuthorID:     st.AuthorID,
		AmountCents:  payable,
		RefCode:      "PAY-" + strings.ToUpper(hex.EncodeToString(b)),
		Note:         "本地模拟付款回执（无真实资金通道）",
	}
	if err := s.db.Create(&receipt).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	st.Status = StatusPaid
	st.PaidAt = &now
	s.db.Save(&st)
	c.JSON(200, gin.H{"ok": true, "receipt": receipt})
}

// listExceptions 异常销量清单（跨区/跨期退货/无合同），财务核对用
func (s *Server) listExceptions(c *gin.Context) {
	u := currentUser(c)
	var records []SalesRecord
	s.db.Order("sale_date, id").Find(&records)
	type exItem struct {
		SalesRecord
		WorkTitle string `json:"work_title"`
		Category  string `json:"category"`
		Reason    string `json:"reason"`
	}
	var out []exItem
	works := map[uint]string{}
	for _, r := range records {
		plan, err := classifyRecord(s.db, r)
		if err != nil || plan.category == CatNormal {
			continue
		}
		if u.Role == "author" {
			owns := false
			for _, sh := range plan.shares {
				if u.AuthorID != nil && sh.authorID == *u.AuthorID {
					owns = true
				}
			}
			if !owns {
				continue
			}
		}
		title, ok := works[r.WorkID]
		if !ok {
			var w Work
			s.db.First(&w, r.WorkID)
			title = w.Title
			works[r.WorkID] = title
		}
		out = append(out, exItem{SalesRecord: r, WorkTitle: title, Category: plan.category, Reason: plan.reason})
	}
	c.JSON(200, out)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
