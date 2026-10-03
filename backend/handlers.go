package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Router(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/auth/login", loginHandler(db))

	api := r.Group("/api", authMiddleware(db))

	// 通用资料（所有登录角色可查）
	api.GET("/works", listWorks(db))
	api.GET("/works/:id", getWork(db))
	api.GET("/works/:id/license", licenseQuery(db))
	api.GET("/contracts/:id", getContract(db))
	api.GET("/authors", listAuthors(db))

	// 编辑：作品 / 作者 / 合同版本
	editor := api.Group("", requireRole("editor"))
	editor.POST("/works", createWork(db))
	editor.POST("/authors", createAuthor(db))
	editor.POST("/works/:id/contracts", createContractVersion(db))

	// 财务
	fin := api.Group("", requireRole("finance"))
	fin.POST("/sales/import", importSales(db))
	fin.GET("/batches", listBatches(db))
	fin.GET("/batches/:id", getBatch(db))
	fin.GET("/sales/lines", listSalesLines(db))
	fin.POST("/sales/lines/:id/resolve", resolveException(db))
	fin.POST("/settlements/generate", generateSettlementsAPI(db))
	fin.GET("/settlements", listSettlements(db))
	fin.POST("/settlements/:id/confirm", confirmSettlement(db))
	fin.POST("/adjustments/:id/confirm", confirmAdjustment(db))
	fin.POST("/adjustments", createAdjustment(db))
	fin.POST("/exceptions/:lineId/allow", allowException(db))
	fin.GET("/adjustments", listAdjustments(db))
	fin.POST("/payments/pay", pay(db))
	fin.GET("/payments", listPayments(db))
	fin.GET("/finance/overview", overview(db))

	// 结算单详情财务可见；作者在 my 命名空间下只看自己的
	fin.GET("/settlements/:id", getSettlement(db))
	fin.GET("/adjustments/:id", getAdjustment(db))

	me := api.Group("/my", requireRole("author"))
	me.GET("/profile", myProfile(db))
	me.GET("/settlements", mySettlements(db))
	me.GET("/settlements/:id", mySettlementDetail(db))
	me.GET("/adjustments", myAdjustments(db))
	me.GET("/summary", mySummary(db))

	return r
}

// ---------- 认证 ----------

func loginHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var u User
		if err := db.Where("email = ?", strings.TrimSpace(in.Email)).First(&u).Error; err != nil {
			c.JSON(401, gin.H{"error": "邮箱或密码错误"})
			return
		}
		if hashPassword(in.Password, u.Salt) != u.PasswordHash {
			c.JSON(401, gin.H{"error": "邮箱或密码错误"})
			return
		}
		c.JSON(200, gin.H{
			"token": issueToken(u),
			"user": gin.H{
				"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role,
				"author_id": u.AuthorID,
			},
		})
	}
}

// ---------- 作品 / 作者 ----------

func listWorks(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var works []Work
		q := db.Model(&Work{})
		if kw := c.Query("q"); kw != "" {
			q = q.Where("title LIKE ? OR isbn LIKE ?", "%"+kw+"%", "%"+kw+"%")
		}
		q.Order("id").Find(&works)
		c.JSON(200, gin.H{"items": works})
	}
}

func createWork(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			Title       string `json:"title" binding:"required"`
			ISBN        string `json:"isbn"`
			Description string `json:"description"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		w := Work{Title: in.Title, ISBN: strings.TrimSpace(in.ISBN), Description: in.Description}
		if err := db.Create(&w).Error; err != nil {
			c.JSON(400, gin.H{"error": "ISBN 重复或创建失败"})
			return
		}
		c.JSON(200, w)
	}
}

func listAuthors(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var authors []Author
		db.Order("id").Find(&authors)
		c.JSON(200, gin.H{"items": authors})
	}
}

func createAuthor(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			Name     string `json:"name" binding:"required"`
			Email    string `json:"email" binding:"required"`
			BankInfo string `json:"bank_info"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var exists int64
		db.Model(&Author{}).Where("email = ?", in.Email).Count(&exists)
		if exists > 0 {
			c.JSON(400, gin.H{"error": "作者邮箱已存在"})
			return
		}
		var a Author
		err := db.Transaction(func(tx *gorm.DB) error {
			a = Author{Name: in.Name, Email: in.Email, BankInfo: in.BankInfo}
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
			if in.Password != "" {
				createUser(tx, in.Email, in.Password, in.Name, "author", &a.ID)
			}
			return nil
		})
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, a)
	}
}

func getWork(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var w Work
		if err := db.First(&w, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "作品不存在"})
			return
		}
		var versions []ContractVersion
		db.Where("work_id = ?", id).Order("version_no DESC").Find(&versions)
		for i := range versions {
			db.Preload("Author").Where("contract_id = ?", versions[i].ID).Order("id").Find(&versions[i].Shares)
		}
		c.JSON(200, gin.H{"work": w, "versions": versions})
	}
}

// 授权范围直接查询：给定日期/区域/语种，返回命中的合同版本与是否在授权内
func licenseQuery(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
		region := strings.ToUpper(c.Query("region"))
		language := strings.ToLower(c.Query("language"))

		cv, shares, ok := findContractAt(db, uint(id), date)
		resp := gin.H{"date": date, "region": region, "language": language, "licensed": false}
		if !ok {
			resp["reason"] = "该日期无有效合同版本"
			c.JSON(200, resp)
			return
		}
		regionOK := region == "" || inList(cv.Regions, region)
		languageOK := language == "" || inList(cv.Languages, language)
		resp["licensed"] = regionOK && languageOK
		resp["contract"] = cv
		resp["shares"] = shares
		resp["region_covered"] = regionOK
		resp["language_covered"] = languageOK
		if !regionOK {
			resp["reason"] = fmt.Sprintf("区域 %s 不在授权区域 %s 内（越区）", region, cv.Regions)
		} else if !languageOK {
			resp["reason"] = fmt.Sprintf("语种 %s 不在授权语种 %s 内", language, cv.Languages)
		}
		c.JSON(200, resp)
	}
}

func getContract(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var cv ContractVersion
		if err := db.First(&cv, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "合同版本不存在"})
			return
		}
		db.Preload("Author").Where("contract_id = ?", id).Order("id").Find(&cv.Shares)
		c.JSON(200, cv)
	}
}

// ---------- 合同版本 ----------

type shareInput struct {
	AuthorID uint  `json:"author_id"`
	ShareBps int64 `json:"share_bps"`
}

func createContractVersion(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		workID, _ := strconv.Atoi(c.Param("id"))
		var w Work
		if err := db.First(&w, workID).Error; err != nil {
			c.JSON(404, gin.H{"error": "作品不存在"})
			return
		}
		var in struct {
			EffectiveFrom string       `json:"effective_from" binding:"required"`
			Languages     string       `json:"languages" binding:"required"`
			Regions       string       `json:"regions" binding:"required"`
			PrintRateType string       `json:"print_rate_type"`
			PrintRate     int64        `json:"print_rate"`
			EbookRateType string       `json:"ebook_rate_type"`
			EbookRate     int64        `json:"ebook_rate"`
			Note          string       `json:"note"`
			Shares        []shareInput `json:"shares"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if len(in.EffectiveFrom) != 10 {
			c.JSON(400, gin.H{"error": "effective_from 必须为 YYYY-MM-DD"})
			return
		}
		if in.PrintRateType == "" {
			in.PrintRateType = "percent"
		}
		if in.EbookRateType == "" {
			in.EbookRateType = "percent"
		}
		if in.PrintRateType != "percent" && in.PrintRateType != "per_copy" {
			c.JSON(400, gin.H{"error": "print_rate_type 必须为 percent/per_copy"})
			return
		}
		if len(in.Shares) == 0 {
			c.JSON(400, gin.H{"error": "至少一位作者分成"})
			return
		}
		var sum int64
		seen := map[uint]bool{}
		for _, s := range in.Shares {
			if s.AuthorID == 0 || s.ShareBps <= 0 {
				c.JSON(400, gin.H{"error": "分成作者与比例必须大于 0"})
				return
			}
			if seen[s.AuthorID] {
				c.JSON(400, gin.H{"error": "同一作者不能重复出现"})
				return
			}
			seen[s.AuthorID] = true
			var a Author
			if err := db.First(&a, s.AuthorID).Error; err != nil {
				c.JSON(400, gin.H{"error": fmt.Sprintf("作者 %d 不存在", s.AuthorID)})
				return
			}
			sum += s.ShareBps
		}
		if sum != 10000 {
			c.JSON(400, gin.H{"error": fmt.Sprintf("分成比例合计必须为 100%%（10000 bps），当前 %d bps（%.2f%%）", sum, float64(sum)/100)})
			return
		}

		var cv ContractVersion
		err := db.Transaction(func(tx *gorm.DB) error {
			var maxNo int
			tx.Model(&ContractVersion{}).Where("work_id = ?", workID).
				Select("COALESCE(MAX(version_no),0)").Scan(&maxNo)

			// 区间重叠检查：新版本 [from, 2999-12-31)。
			// 若 from 落在某旧版本区间内 → 自动截断旧版本到 from（新版本生效，旧版终止）；
			// 若与更早的封闭版本发生其他重叠 → 拒绝。
			var old []ContractVersion
			tx.Where("work_id = ?", workID).Find(&old)
			for _, o := range old {
				if in.EffectiveFrom < o.EffectiveFrom {
					return fmt.Errorf("新生效日 %s 早于 v%d 的生效日 %s，区间重叠", in.EffectiveFrom, o.VersionNo, o.EffectiveFrom)
				}
				if in.EffectiveFrom >= o.EffectiveFrom && in.EffectiveFrom < o.EffectiveTo {
					// 正常截断（旧版本从此日终止）
					if err := tx.Model(&ContractVersion{}).Where("id = ?", o.ID).
						Update("effective_to", in.EffectiveFrom).Error; err != nil {
						return err
					}
				}
			}

			cv = ContractVersion{
				WorkID: uint(workID), VersionNo: maxNo + 1,
				EffectiveFrom: in.EffectiveFrom, EffectiveTo: "2999-12-31",
				Languages: strings.ToLower(in.Languages), Regions: strings.ToUpper(in.Regions),
				PrintRateType: in.PrintRateType, PrintRate: in.PrintRate,
				EbookRateType: in.EbookRateType, EbookRate: in.EbookRate, Note: in.Note,
			}
			if err := tx.Create(&cv).Error; err != nil {
				return err
			}
			for _, s := range in.Shares {
				if err := tx.Create(&ContractShare{ContractID: cv.ID, AuthorID: s.AuthorID, ShareBps: s.ShareBps}).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		db.Preload("Author").Where("contract_id = ?", cv.ID).Find(&cv.Shares)
		c.JSON(200, cv)
	}
}

// ---------- 销量 ----------

func importSales(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in ImportBatchInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		res, err := ImportBatch(db, in)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, res)
	}
}

func listBatches(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var batches []SalesBatch
		q := db.Model(&SalesBatch{})
		if p := c.Query("period"); p != "" {
			q = q.Where("period = ?", p)
		}
		q.Order("id DESC").Find(&batches)
		c.JSON(200, gin.H{"items": batches})
	}
}

func getBatch(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var b SalesBatch
		if err := db.First(&b, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "批次不存在"})
			return
		}
		db.Order("id").Find(&b.Lines, "batch_id = ?", id)
		c.JSON(200, b)
	}
}

func listSalesLines(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var lines []SalesLine
		q := db.Model(&SalesLine{})
		if p := c.Query("period"); p != "" {
			q = q.Where("batch_id IN (SELECT id FROM sales_batches WHERE period = ?)", p)
		}
		if s := c.Query("status"); s != "" {
			q = q.Where("status = ?", s)
		}
		if e := c.Query("exception_type"); e != "" {
			q = q.Where("exception_type = ?", e)
		}
		if w := c.Query("work_id"); w != "" {
			q = q.Where("work_id = ?", w)
		}
		q.Order("id DESC").Limit(1000).Find(&lines)
		c.JSON(200, gin.H{"items": lines})
	}
}

func resolveException(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var line SalesLine
		if err := db.First(&line, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "销量不存在"})
			return
		}
		var in struct {
			Memo string `json:"memo"`
		}
		_ = c.ShouldBindJSON(&in)
		db.Model(&line).Updates(map[string]any{"resolved": true, "exception_memo": line.ExceptionMemo + " | 已处理:" + in.Memo})
		c.JSON(200, gin.H{"ok": true})
	}
}

// ---------- 结算 ----------

func generateSettlementsAPI(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			Period string `json:"period" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || len(in.Period) != 7 {
			c.JSON(400, gin.H{"error": "period 必填（YYYY-MM）"})
			return
		}
		affected, err := GenerateSettlements(db, in.Period)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		// 统计期内异常数
		var exCount int64
		db.Model(&SalesLine{}).
			Where("status = ? AND batch_id IN (SELECT id FROM sales_batches WHERE period = ?)", lineException, in.Period).
			Count(&exCount)
		c.JSON(200, gin.H{"period": in.Period, "affected_authors": affected, "exception_lines": exCount})
	}
}

type settlementDTO struct {
	Settlement
	WorkAmounts     map[uint]int64 `json:"work_amounts"`
	AdjustmentTotal int64          `json:"adjustment_total"`
	PayableTotal    int64          `json:"payable_total"`
	RelatedAdjusts  []Adjustment   `json:"related_adjustments"`
}

func loadSettlementDTO(db *gorm.DB, st Settlement) settlementDTO {
	db.Preload("Author").First(&st, st.ID)
	db.Preload("Receipt").First(&st, st.ID)
	db.Order("id").Find(&st.Entries, "settlement_id = ?", st.ID)
	dto := settlementDTO{Settlement: st, WorkAmounts: map[uint]int64{}}
	for _, e := range st.Entries {
		dto.WorkAmounts[e.WorkID] += e.Royalty
	}
	var adjusts []Adjustment
	db.Where("original_settlement_id = ?", st.ID).Order("id").Find(&adjusts)
	dto.RelatedAdjusts = adjusts
	for _, a := range adjusts {
		dto.AdjustmentTotal += a.Amount
	}
	if st.Status == "draft" {
		dto.PayableTotal = st.NormalAmount
	} else {
		dto.PayableTotal = st.NormalAmount + dto.AdjustmentTotal
	}
	return dto
}

func listSettlements(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var sts []Settlement
		q := db.Model(&Settlement{})
		if p := c.Query("period"); p != "" {
			q = q.Where("period = ?", p)
		}
		if s := c.Query("status"); s != "" {
			q = q.Where("status = ?", s)
		}
		if a := c.Query("author_id"); a != "" {
			q = q.Where("author_id = ?", a)
		}
		q.Order("period DESC, id").Find(&sts)
		out := []settlementDTO{}
		for _, st := range sts {
			out = append(out, loadSettlementDTO(db, st))
		}
		c.JSON(200, gin.H{"items": out})
	}
}

func getSettlement(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var st Settlement
		if err := db.First(&st, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "结算单不存在"})
			return
		}
		c.JSON(200, loadSettlementDTO(db, st))
	}
}

// ---------- 补差单 ----------

type adjEntryInput struct {
	SalesLineID *uint  `json:"sales_line_id"`
	WorkID      uint   `json:"work_id"`
	ContractID  uint   `json:"contract_id"`
	Memo        string `json:"memo"`
	Amount      int64  `json:"amount"`
}

func createAdjustment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			OriginalSettlementID *uint           `json:"original_settlement_id"`
			AuthorID             uint            `json:"author_id" binding:"required"`
			Period               string          `json:"period" binding:"required"`
			Reason               string          `json:"reason" binding:"required"`
			Memo                 string          `json:"memo"`
			Entries              []adjEntryInput `json:"entries"`
			Amount               *int64          `json:"amount"` // 无明细时直接给金额（差错更正）
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if len(in.Period) != 7 {
			c.JSON(400, gin.H{"error": "period 必须为 YYYY-MM"})
			return
		}
		var author Author
		if err := db.First(&author, in.AuthorID).Error; err != nil {
			c.JSON(400, gin.H{"error": "作者不存在"})
			return
		}
		var total int64
		for _, e := range in.Entries {
			total += e.Amount
		}
		if len(in.Entries) == 0 {
			if in.Amount == nil {
				c.JSON(400, gin.H{"error": "请提供 entries 或 amount"})
				return
			}
			total = *in.Amount
		}
		if total == 0 {
			c.JSON(400, gin.H{"error": "补差金额合计不能为 0"})
			return
		}
		if in.OriginalSettlementID != nil {
			var orig Settlement
			if err := db.First(&orig, *in.OriginalSettlementID).Error; err != nil {
				c.JSON(400, gin.H{"error": "原结算单不存在"})
				return
			}
			if orig.Status == "draft" {
				c.JSON(400, gin.H{"error": "原结算单还是草稿，请直接在草稿上重算，无需补差单"})
				return
			}
			if orig.AuthorID != in.AuthorID {
				c.JSON(400, gin.H{"error": "补差单作者必须与原结算单一致"})
				return
			}
		}

		var adj Adjustment
		err := db.Transaction(func(tx *gorm.DB) error {
			adj = Adjustment{
				Code:                 fmt.Sprintf("ADJ-%s-A%d-%d", in.Period, in.AuthorID, time.Now().UnixNano()%100000),
				OriginalSettlementID: in.OriginalSettlementID,
				AuthorID:             in.AuthorID, Period: in.Period, Reason: in.Reason, Memo: in.Memo,
				Amount: total, Status: "draft",
			}
			if err := tx.Create(&adj).Error; err != nil {
				return err
			}
			if len(in.Entries) > 0 {
				for _, e := range in.Entries {
					if err := tx.Create(&AdjustmentEntry{
						AdjustmentID: adj.ID, SalesLineID: e.SalesLineID, WorkID: e.WorkID,
						ContractID: e.ContractID, Memo: e.Memo, Amount: e.Amount,
					}).Error; err != nil {
						return err
					}
				}
			} else {
				if err := tx.Create(&AdjustmentEntry{
					AdjustmentID: adj.ID, WorkID: 0, Memo: in.Memo, Amount: total,
				}).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		db.Preload("Entries").First(&adj, adj.ID)
		c.JSON(200, adj)
	}
}

// allowException 把一条异常销量按指定合同版本特批计酬，
// 为每位（或指定）作者生成一张关联当期原单的补差单 —— 异常收入永不混入正常收入。
func allowException(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lineID, _ := strconv.Atoi(c.Param("lineId"))
		var line SalesLine
		if err := db.First(&line, lineID).Error; err != nil {
			c.JSON(404, gin.H{"error": "销量不存在"})
			return
		}
		if line.Status != lineException {
			c.JSON(400, gin.H{"error": "该行不是异常销量"})
			return
		}
		var in struct {
			Period    string `json:"period"`
			Memo      string `json:"memo"`
			AuthorIDs []uint `json:"author_ids"`
			RateBps   *int64 `json:"rate_bps"` // 可覆盖费率（如无合同特批）
		}
		_ = c.ShouldBindJSON(&in)
		if in.Period == "" {
			in.Period = periodOf(line.SaleDate)
		}

		// 选定计酬依据：行内已记录的合同版本优先（跨区/跨期退货），否则取最新版本
		cvID := line.ContractID
		var cv ContractVersion
		var shares []ContractShare
		if cvID != 0 {
			db.First(&cv, cvID)
			db.Preload("Author").Where("contract_id = ?", cvID).Order("id").Find(&shares)
		}
		if cvID == 0 {
			var ok bool
			cv, shares, ok = latestContract(db, line.WorkID)
			if !ok {
				c.JSON(400, gin.H{"error": "该作品没有任何合同，无法特批；请手工开差错补差单"})
				return
			}
		}
		gross := line.RoyaltyGross
		if in.RateBps != nil && *in.RateBps > 0 {
			gross = divBps(line.Amount, *in.RateBps)
		}
		if gross == 0 && line.Amount != 0 {
			// 到期补传且行上没有合同：用最新合同费率重算
			gross = calcGross(&line, cv)
		}

		want := map[uint]bool{}
		for _, a := range in.AuthorIDs {
			want[a] = true
		}
		bpsList := []int64{}
		kept := []ContractShare{}
		for _, s := range shares {
			if len(want) == 0 || want[s.AuthorID] {
				bpsList = append(bpsList, s.ShareBps)
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			c.JSON(400, gin.H{"error": "指定的作者不在合同分成名单中"})
			return
		}
		parts := splitByBps(gross, bpsList)

		reasonMap := map[string]string{
			exCrossRegion:       "exception_allowed",
			exCrossPeriodReturn: "cross_period_return",
			exPostExpiry:        "late_sales",
			exLanguage:          "exception_allowed",
			exNoContract:        "late_sales",
			exSettlementLocked:  "late_sales",
		}
		created := []Adjustment{}
		err := db.Transaction(func(tx *gorm.DB) error {
			for i, s := range kept {
				// 关联该作者当期原结算单（若存在且已确认）
				var origID *uint
				var orig Settlement
				if err := tx.Where("period = ? AND author_id = ?", in.Period, s.AuthorID).First(&orig).Error; err == nil {
					if orig.Status != "draft" {
						id := orig.ID
						origID = &id
					}
				}
				lineID := uint(line.ID)
				adj := Adjustment{
					Code:                 fmt.Sprintf("ADJ-%s-A%d-%d", in.Period, s.AuthorID, int(line.ID)*100+i+1),
					OriginalSettlementID: origID,
					AuthorID:             s.AuthorID, Period: in.Period,
					Reason: reasonMap[line.ExceptionType], Memo: in.Memo,
					Amount: parts[i], Status: "draft",
				}
				if adj.Reason == "" {
					adj.Reason = "exception_allowed"
				}
				if err := tx.Create(&adj).Error; err != nil {
					return err
				}
				if err := tx.Create(&AdjustmentEntry{
					AdjustmentID: adj.ID, SalesLineID: &lineID, WorkID: line.WorkID,
					ContractID: cv.ID, Amount: parts[i],
					Memo: fmt.Sprintf("异常特批[%s]：%s（合同v%d，分成%.2f%%）", line.ExceptionType, line.ExceptionMemo, cv.VersionNo, float64(s.ShareBps)/100),
				}).Error; err != nil {
					return err
				}
				created = append(created, adj)
			}
			return tx.Model(&line).Update("resolved", true).Error
		})
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"gross": gross, "adjustments": created})
	}
}

func listAdjustments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var adjs []Adjustment
		q := db.Model(&Adjustment{})
		if p := c.Query("period"); p != "" {
			q = q.Where("period = ?", p)
		}
		if a := c.Query("author_id"); a != "" {
			q = q.Where("author_id = ?", a)
		}
		q.Order("id DESC").Find(&adjs)
		out := make([]Adjustment, 0, len(adjs))
		for _, a := range adjs {
			db.Preload("Author").Preload("Receipt").First(&a, a.ID)
			db.Order("id").Find(&a.Entries, "adjustment_id = ?", a.ID)
			out = append(out, a)
		}
		c.JSON(200, gin.H{"items": out})
	}
}

func getAdjustment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var a Adjustment
		if err := db.First(&a, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "补差单不存在"})
			return
		}
		db.Preload("Author").Preload("Receipt").First(&a, a.ID)
		db.Order("id").Find(&a.Entries, "adjustment_id = ?", a.ID)
		c.JSON(200, a)
	}
}

// ---------- 确认 / 模拟付款 ----------

func confirmable(tx *gorm.DB, kind string, id uint) (int64, uint, error) {
	switch kind {
	case "settlement":
		var st Settlement
		if err := tx.First(&st, id).Error; err != nil {
			return 0, 0, err
		}
		if st.Status != "confirmed" {
			return 0, 0, fmt.Errorf("结算单当前状态 %s：需先确认（冻结金额）后才能付款", st.Status)
		}
		return st.NormalAmount, st.AuthorID, nil
	case "adjustment":
		var a Adjustment
		if err := tx.First(&a, id).Error; err != nil {
			return 0, 0, err
		}
		if a.Status != "confirmed" {
			return 0, 0, fmt.Errorf("补差单当前状态 %s：需先确认后才能付款", a.Status)
		}
		return a.Amount, a.AuthorID, nil
	}
	return 0, 0, errors.New("type 必须为 settlement/adjustment")
}

func pay(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct {
			Type string `json:"type" binding:"required"`
			ID   uint   `json:"id" binding:"required"`
			Memo string `json:"memo"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var receipt PaymentReceipt
		err := db.Transaction(func(tx *gorm.DB) error {
			// 幂等：已有回执直接返回
			if err := tx.Where("payable_type = ? AND payable_id = ?", in.Type, in.ID).First(&receipt).Error; err == nil {
				return nil
			}
			amount, authorID, err := confirmable(tx, in.Type, in.ID)
			if err != nil {
				return err
			}
			now := time.Now()
			if in.Type == "settlement" {
				if err := tx.Model(&Settlement{}).Where("id = ?", in.ID).
					Updates(map[string]any{"status": "paid", "confirmed_at": now, "paid_at": now}).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&Adjustment{}).Where("id = ?", in.ID).
					Updates(map[string]any{"status": "paid", "confirmed_at": now, "paid_at": now}).Error; err != nil {
					return err
				}
			}
			var author Author
			tx.First(&author, authorID)
			receipt = PaymentReceipt{
				PayableType: in.Type, PayableID: in.ID,
				ReceiptNo: fmt.Sprintf("RC-%s-%d-%d", strings.ToUpper(in.Type[:3]), in.ID, now.UnixNano()%1000000),
				Channel:   "本地模拟银行通道", Amount: amount, PaidAt: now,
				Memo: fmt.Sprintf("收款人：%s（%s）%s", author.Name, author.BankInfo, in.Memo),
			}
			return tx.Create(&receipt).Error
		})
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, receipt)
	}
}

func listPayments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var rs []PaymentReceipt
		db.Order("id DESC").Find(&rs)
		c.JSON(200, gin.H{"items": rs})
	}
}
