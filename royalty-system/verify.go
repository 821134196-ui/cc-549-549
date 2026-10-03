package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type check struct {
	name   string
	ok     bool
	detail string
}

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	keep := fs.Bool("keep", false, "保留临时数据库")
	_ = fs.Parse(args)

	tmp, err := os.MkdirTemp("", "royalty-verify-")
	must(err)
	dbPath := filepath.Join(tmp, "verify.db")
	if *keep {
		dbPath = "data/verify.db"
		_ = os.Remove(dbPath)
		fmt.Println("（--keep 模式，数据库保留在", dbPath, "）")
	} else {
		defer os.RemoveAll(tmp)
	}

	db, err := openDB(dbPath)
	must(err)
	must(SeedMasterData(db))

	var checks []check
	assert := func(name string, cond bool, detail string) {
		checks = append(checks, check{name, cond, detail})
	}

	importFile := func(name string) (*ImportResult, error) {
		data, err := sampleFS.ReadFile("data/" + name)
		if err != nil {
			return nil, err
		}
		return ImportSales(db, name, bytes.NewReader(data))
	}

	// ---------- 第一次导入 6 月 ----------
	res1, err := importFile("sales-2025-06.csv")
	must(err)
	recs, lines, err := RegenerateSettlements(db)
	must(err)
	assert("6月销量导入5行", res1.ImportedRows == 5, fmt.Sprintf("导入 %d 行", res1.ImportedRows))
	assert("重算扫描5条销量、生成10条作者明细(5×2作者)", recs == 5 && lines == 10,
		fmt.Sprintf("扫描 %d 条，明细 %d 条", recs, lines))

	findSettlement := func(authorCode, period, kind string) *Settlement {
		var a Author
		if err := db.Where("code = ?", authorCode).First(&a).Error; err != nil {
			return nil
		}
		var st Settlement
		if err := db.Where("author_id = ? AND period = ? AND kind = ?", a.ID, period, kind).First(&st).Error; err != nil {
			return nil
		}
		return &st
	}
	wangJun := findSettlement("A001", "2025-06", KindNormal)
	liJun := findSettlement("A002", "2025-06", KindNormal)

	// S001 5000×10%=500.00；S002 4000×15%=600.00；S004 -500×10%=-50.00；S005 33.37×10%=3.34（最大余数法 2.00+1.34）
	wantWang := int64(30000 + 36000 - 3000 + 200) // 632.00
	wantLi := int64(20000 + 24000 - 2000 + 134)   // 421.34
	assert("多作者分成-王明60%应付632.00", wangJun != nil && wangJun.NormalAmountCents == wantWang,
		fmt.Sprintf("实际 %d 分", nz(wangJun)))
	assert("多作者分成-李华40%应付421.34（分币无尾差）", liJun != nil && liJun.NormalAmountCents == wantLi,
		fmt.Sprintf("实际 %d 分", nz(liJun)))
	assert("两位作者合计恰为版税总额1053.34",
		wangJun.NormalAmountCents+liJun.NormalAmountCents == 105334,
		fmt.Sprintf("合计 %d 分", wangJun.NormalAmountCents+liJun.NormalAmountCents))

	// S003 台湾区域：v1 仅授权 CN → 跨区销售，隔离不进正常收入
	assert("跨区销售(TW)隔离-王明异常90.00", wangJun.ExceptionAmountCents == 9000,
		fmt.Sprintf("异常 %d 分", wangJun.ExceptionAmountCents))
	assert("跨区销售(TW)隔离-李华异常60.00", liJun.ExceptionAmountCents == 6000,
		fmt.Sprintf("异常 %d 分", liJun.ExceptionAmountCents))

	var s003line SettlementLine
	db.Where("sales_record_id = (SELECT id FROM sales_records WHERE ref='S003') AND author_id = (SELECT id FROM authors WHERE code='A001')").
		First(&s003line)
	assert("S003分类为cross_region", s003line.Category == CatCrossRegion, s003line.Category+" "+s003line.Reason)

	// 同期退货 S004 正常抵扣
	var s004line SettlementLine
	db.Where("sales_record_id = (SELECT id FROM sales_records WHERE ref='S004') AND author_id = (SELECT id FROM authors WHERE code='A001')").
		First(&s004line)
	assert("同期退货S004按normal抵扣", s004line.Category == CatNormal && s004line.AmountCents == -3000,
		fmt.Sprintf("%s %d", s004line.Category, s004line.AmountCents))

	// 明细可追溯到原始销量与合同版本
	assert("结算行关联合同版本v1", s004line.ContractVersionID != nil, "")
	var v1 ContractVersion
	db.Where("work_id = (SELECT id FROM works WHERE code='W001') AND version_no = 1").First(&v1)
	assert("结算行指向销售时生效的v1版本", s004line.ContractVersionID != nil && *s004line.ContractVersionID == v1.ID, "")

	// ---------- 幂等：重复导入同一文件 ----------
	_, err = importFile("sales-2025-06.csv")
	assert("重复导入同一批文件被拒绝", err != nil && strings.Contains(err.Error(), "重复导入"),
		errStr(err))

	// 同 ref 不同文件：批次哈希不同，但行级唯一索引拦截
	resDup, err := importFile("sales-dup-test.csv")
	assert("同渠道行号不同文件不重复计酬(0入账1跳过)",
		err == nil && resDup.ImportedRows == 0 && resDup.SkippedDup == 1,
		fmt.Sprintf("入账 %d 跳过 %d err=%v", resDup.ImportedRows, resDup.SkippedDup, err))
	n2r, n2l, _ := RegenerateSettlements(db)
	assert("重复行重算后仍只有5条销量、10条作者明细", n2r == 5 && n2l == 10,
		fmt.Sprintf("扫描 %d 条销量、%d 条明细", n2r, n2l))
	var salesCount int64
	db.Model(&SalesRecord{}).Count(&salesCount)
	assert("销量表仍为5条（重复批次未落任何行）", salesCount == 5, fmt.Sprintf("%d 条", salesCount))

	// 重算会重建草稿单，确认前重新定位结算单 ID
	wangJun = findSettlement("A001", "2025-06", KindNormal)
	liJun = findSettlement("A002", "2025-06", KindNormal)

	// ---------- 启动 HTTP 服务验证角色权限与确认/补差/付款 ----------
	gin.SetMode(gin.TestMode)
	srv := &Server{db: db, auth: newTokenStore()}
	r := gin.New()
	srv.registerRoutes(r, "web")
	do := func(method, path, token string, body any) (int, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	login := func(name, pwd string) string {
		code, out := do("POST", "/api/login", "", map[string]string{"login_name": name, "password": pwd})
		if code != 200 {
			return ""
		}
		return out["token"].(string)
	}
	financeToken := login("finance", "finance123")
	wangToken := login("wangming", "author123")
	editorToken := login("editor", "editor123")
	assert("三类角色均可登录", financeToken != "" && wangToken != "" && editorToken != "", "")

	// 作者无权导入销量
	code, _ := do("POST", "/api/sales/import", wangToken, nil)
	assert("作者不能导入销量(403)", code == 403, fmt.Sprintf("状态码 %d", code))
	code, _ = do("GET", "/api/settlements", editorToken, nil)
	assert("编辑可浏览结算列表(200)", code == 200, fmt.Sprintf("状态码 %d", code))

	// 确认王明 6 月正常结算单
	code, _ = do("POST", fmt.Sprintf("/api/settlements/%d/confirm", wangJun.ID), financeToken, nil)
	assert("财务确认王明6月结算单", code == 200, fmt.Sprintf("状态码 %d", code))
	var wangJunAfter Settlement
	db.First(&wangJunAfter, wangJun.ID)
	assert("确认后状态=confirmed", wangJunAfter.Status == StatusConfirmed, wangJunAfter.Status)

	// 已确认单不能再确认/改金额：再次确认报错
	code, out := do("POST", fmt.Sprintf("/api/settlements/%d/confirm", wangJun.ID), financeToken, nil)
	assert("已确认单拒绝再次确认(409)", code == 409, fmt.Sprintf("%v", out["error"]))

	// ---------- 第二次导入 7 月：跨期退货、到期补传、确认后补录 ----------
	res2, err := importFile("sales-2025-07.csv")
	must(err)
	assert("7月销量导入5行", res2.ImportedRows == 5, fmt.Sprintf("导入 %d 行", res2.ImportedRows))
	n3r, n3l, err := RegenerateSettlements(db)
	must(err)
	// 王明6月5条明细已锁定；7月文件对王明新增5条(6月补单1+7月3+10月1)，李华全部10条草稿重建 → 5+10=15
	assert("重算扫描10条销量、生成15条作者明细(已确认行锁定)", n3r == 10 && n3l == 15,
		fmt.Sprintf("扫描 %d 条，明细 %d 条", n3r, n3l))

	// 已确认的王明6月金额分文未动
	db.First(&wangJunAfter, wangJun.ID)
	assert("已确认单金额不被重算修改(仍632.00)", wangJunAfter.NormalAmountCents == 63200,
		fmt.Sprintf("金额 %d 分", wangJunAfter.NormalAmountCents))
	assert("已确认单状态仍为confirmed", wangJunAfter.Status == StatusConfirmed, wangJunAfter.Status)

	// 王明补录的6月销量 S103 进入补结算单并关联原单
	wangJunSupp := findSettlement("A001", "2025-06", KindSupplement)
	assert("确认后补录形成补结算单(supplement)", wangJunSupp != nil && wangJunSupp.NormalAmountCents == 6000,
		fmt.Sprintf("补单 %+v", wangJunSupp))
	assert("补结算单关联原结算单", wangJunSupp != nil && wangJunSupp.LinkedSettlementID != nil &&
		*wangJunSupp.LinkedSettlementID == wangJun.ID, "")

	// 李华6月仍是草稿：S103 并入重算（421.34 + 40.00 = 461.34）
	liJun2 := findSettlement("A002", "2025-06", KindNormal)
	assert("草稿单随补录重算-李华6月461.34", liJun2.NormalAmountCents == 46134,
		fmt.Sprintf("金额 %d 分", liJun2.NormalAmountCents))

	// 7月：S101 正常（v2 英文电子书15%：300.00，王明180/李华120）
	wangJul := findSettlement("A001", "2025-07", KindNormal)
	liJul := findSettlement("A002", "2025-07", KindNormal)
	assert("7月正常收入-王明180.00", wangJul.NormalAmountCents == 18000,
		fmt.Sprintf("%d 分", wangJul.NormalAmountCents))
	assert("7月正常收入-李华120.00", liJul.NormalAmountCents == 12000,
		fmt.Sprintf("%d 分", liJul.NormalAmountCents))

	// 7月异常：S102 HK跨区(240版税:144/96) + S104跨期退货(-30版税:-18/-12，按7月v2费率12%)
	assert("7月异常隔离-王明126.00(跨区144-跨期退货18)", wangJul.ExceptionAmountCents == 14400-1800,
		fmt.Sprintf("%d 分", wangJul.ExceptionAmountCents))
	assert("7月异常隔离-李华84.00(跨区96-跨期退货12)", liJul.ExceptionAmountCents == 9600-1200,
		fmt.Sprintf("%d 分", liJul.ExceptionAmountCents))

	// S105 是10月销售（合同已到期），单列在10月结算单，不混入7月
	wangOct := findSettlement("A001", "2025-10", KindNormal)
	liOct := findSettlement("A002", "2025-10", KindNormal)
	assert("到期后补传销量单列10月单-王明36.00异常", wangOct != nil && wangOct.NormalAmountCents == 0 && wangOct.ExceptionAmountCents == 3600,
		fmt.Sprintf("%+v", wangOct))
	assert("到期后补传销量单列10月单-李华24.00异常", liOct != nil && liOct.ExceptionAmountCents == 2400,
		fmt.Sprintf("%+v", liOct))

	catOf := func(ref, authorCode string) string {
		var ln SettlementLine
		q := db.Where("sales_record_id = (SELECT id FROM sales_records WHERE ref=?) AND author_id = (SELECT id FROM authors WHERE code=?)",
			ref, authorCode).First(&ln)
		if q.Error != nil {
			return "<missing>"
		}
		return ln.Category
	}
	assert("S102(香港)分类cross_region", catOf("S102", "A001") == CatCrossRegion, catOf("S102", "A001"))
	assert("S104(7月退6月销售)分类cross_period_return", catOf("S104", "A001") == CatCrossPeriodReturn, catOf("S104", "A001"))
	assert("S105(10月合同已到期)分类no_contract", catOf("S105", "A001") == CatNoContract, catOf("S105", "A001"))

	// S101 按销售时点 v2 计算（英文在 v2 才授权）
	var s101 SettlementLine
	db.Where("sales_record_id = (SELECT id FROM sales_records WHERE ref='S101') AND author_id = (SELECT id FROM authors WHERE code='A001')").First(&s101)
	var v2 ContractVersion
	db.Where("work_id = (SELECT id FROM works WHERE code='W001') AND version_no = 2").First(&v2)
	assert("S101适用销售时生效的v2版本", s101.ContractVersionID != nil && *s101.ContractVersionID == v2.ID, "")

	// 授权范围查询：7月15日生效 v2（含 TW/en）
	code, outAuth := do("GET", "/api/works/1/authorization?date=2025-07-15", wangToken, nil)
	effOK := false
	if code == 200 {
		if eff, ok := outAuth["effective"].(map[string]any); ok {
			effOK = int(eff["version_no"].(float64)) == 2
		}
	}
	assert("作品页按日期查询授权范围命中v2", code == 200 && effOK, fmt.Sprintf("code=%d", code))
	// 直接用引擎复核 6月1日 → v1
	ec, _ := effectiveContract(db, 1, date("2025-06-01"))
	assert("6月1日授权范围为v1", ec != nil && ec.ID == v1.ID, "")

	// ---------- 补差单 ----------
	code, out = do("POST", fmt.Sprintf("/api/settlements/%d/adjust", wangJun.ID), financeToken,
		map[string]any{"amount_cents": 500, "reason": "6月少计渠道补贴5.00元"})
	assert("对已确认单开具补差单+5.00", code == 200, fmt.Sprintf("code=%d %v", code, out["error"]))
	code, _ = do("POST", fmt.Sprintf("/api/settlements/%d/adjust", wangJul.ID), financeToken,
		map[string]any{"amount_cents": 999, "reason": "草稿单不允许补差"})
	assert("草稿单不能开补差单(409)", code == 409, fmt.Sprintf("code=%d", code))

	// 结算详情：原销量、抵扣依据、补差均可追溯
	code, detailOut := do("GET", fmt.Sprintf("/api/settlements/%d", wangJun.ID), financeToken, nil)
	payable := int64(0)
	hasReason := false
	if code == 200 {
		payable = int64(detailOut["payable_cents"].(float64))
		for _, a := range detailOut["adjustments"].([]any) {
			if a.(map[string]any)["reason"].(string) != "" {
				hasReason = true
			}
		}
	}
	assert("应付=正常632.00+补差5.00=637.00", payable == 63700, fmt.Sprintf("应付 %d 分", payable))
	assert("补差单带原因并关联原单", hasReason, "")

	// ---------- 模拟付款回执 ----------
	code, payOut := do("POST", fmt.Sprintf("/api/settlements/%d/pay", wangJun.ID), financeToken, nil)
	receiptRef := ""
	if code == 200 {
		receiptRef = payOut["receipt"].(map[string]any)["ref_code"].(string)
	}
	assert("模拟付款生成回执", code == 200 && strings.HasPrefix(receiptRef, "PAY-"), receiptRef)
	var paid Settlement
	db.First(&paid, wangJun.ID)
	assert("付款后状态=paid", paid.Status == StatusPaid, paid.Status)
	var receipt PaymentReceipt
	assert("付款回执落库且金额=637.00", db.Where("settlement_id = ?", wangJun.ID).First(&receipt).Error == nil &&
		receipt.AmountCents == 63700, fmt.Sprintf("%+v", receipt))

	// ---------- 作者数据隔离 ----------
	code, mine := do("GET", "/api/settlements", wangToken, nil)
	cnt := 0
	if code == 200 {
		cnt = len(mine["settlements"].([]any))
	}
	var liSettlement Settlement
	db.Where("author_id = (SELECT id FROM authors WHERE code='A002')").First(&liSettlement)
	code, _ = do("GET", fmt.Sprintf("/api/settlements/%d", liSettlement.ID), wangToken, nil)
	assert("作者不能查看他人结算单(403)", code == 403, fmt.Sprintf("code=%d 王明可见单%d张", code, cnt))

	// 合同分成合计校验：尝试创建合计≠100%的合同应被拒
	badContract := map[string]any{
		"effective_from": "2026-01-01", "print_rate_bps": 1000, "ebook_rate_bps": 1500,
		"participants": []map[string]any{{"author_id": 1, "share_bps": 7000}},
		"regions":      []string{"CN"}, "languages": []string{"zh"},
	}
	code, _ = do("POST", "/api/works/1/contracts", editorToken, badContract)
	assert("分成合计≠100%%的合同被拒绝(400)", code == 400, fmt.Sprintf("code=%d", code))

	// 合同 v3（合法）可由编辑创建
	good := map[string]any{
		"effective_from": "2026-01-01", "print_rate_bps": 1100, "ebook_rate_bps": 1600,
		"participants": []map[string]any{
			{"author_id": 1, "share_bps": 7000}, {"author_id": 2, "share_bps": 3000},
		},
		"regions": []string{"CN", "TW", "HK"}, "languages": []string{"zh", "en"},
	}
	code, _ = do("POST", "/api/works/1/contracts", editorToken, good)
	assert("编辑可创建合同v3(版本号自动递增)", code == 200, fmt.Sprintf("code=%d", code))
	var v3count int64
	db.Model(&ContractVersion{}).Where("work_id = 1 AND version_no = 3").Count(&v3count)
	assert("v3已落库", v3count == 1, "")

	// ---------- 汇总输出 ----------
	pass, fail := 0, 0
	for _, c := range checks {
		if c.ok {
			pass++
			fmt.Printf("  ✓ %s\n", c.name)
		} else {
			fail++
			fmt.Printf("  ✗ %s  [%s]\n", c.name, c.detail)
		}
	}
	fmt.Printf("\n结果: %d 通过, %d 失败\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
	fmt.Println("全部业务规则验证通过。")
}

func nz(s *Settlement) int64 {
	if s == nil {
		return -1
	}
	return s.NormalAmountCents
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
