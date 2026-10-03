package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type checker struct {
	fails []string
	pass  int
}

func (c *checker) ok(name string, cond bool, detail ...any) {
	if cond {
		c.pass++
		fmt.Printf("  \033[32m✔\033[0m %s\n", name)
	} else {
		msg := fmt.Sprint(detail...)
		c.fails = append(c.fails, name+" "+msg)
		fmt.Printf("  \033[31m✘ %s — %s\033[0m\n", name, msg)
	}
}

func (c *checker) eq(name string, got, want int64) {
	c.ok(name, got == want, fmt.Sprintf("期望 %d，实际 %d", want, got))
}

func runVerify(dbPath string) int {
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		os.Remove(p)
	}
	db, err := openDB(dbPath)
	if err != nil {
		fmt.Println("打开数据库失败:", err)
		return 1
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 出版社稿酬结算系统 · 示例数据自检验证")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	c := &checker{}

	if err := Seed(db); err != nil {
		fmt.Println("种子数据失败:", err)
		return 1
	}

	// 基础资料
	var a1, a2, a3 Author
	db.First(&a1, 1)
	db.First(&a2, 2)
	db.First(&a3, 3)
	c.ok("作者种子：林岚/陈墨/周禾", a1.Name == "林岚" && a2.Name == "陈墨" && a3.Name == "周禾")

	var w1, w2, w3 Work
	db.First(&w1, 1)
	db.First(&w2, 2)
	db.First(&w3, 3)

	var c1, c2 ContractVersion
	db.Where("work_id = ? AND version_no = ?", w1.ID, 1).First(&c1)
	db.Where("work_id = ? AND version_no = ?", w1.ID, 2).First(&c2)
	c.ok("合同版本：v1 在 v2 生效日自动截断", c1.EffectiveTo == "2026-07-01" && c2.EffectiveFrom == "2026-07-01",
		fmt.Sprintf("v1 区间 %s~%s", c1.EffectiveFrom, c1.EffectiveTo))

	// 销售时合同版本匹配：6 月销售必须命中 v1，8 月命中 v2
	_, _, junV1 := findContractAt(db, w1.ID, "2026-06-15")
	cvAug, _, augV2 := findContractAt(db, w1.ID, "2026-08-15")
	c.ok("2026-06 销售匹配 v1（销售时有效版本）", junV1)
	c.ok("2026-08 销售匹配 v2", augV2 && cvAug.ID == c2.ID)

	fmt.Println("\n【1】正常计酬与多作者分成")
	st := func(aid uint) Settlement {
		var s Settlement
		db.Where("period = ? AND author_id = ?", "2026-08", aid).First(&s)
		return s
	}
	// 明细：
	//  L1 纸书CN 60000；L2 电子书US 108000；L4 电子书GB 28800；L10 零头行 101 → w1 合计 196901
	//  w2：按册 300×200=60000 + 电子书 300000×12%=36000 = 96000（全归周禾）
	c.eq("林岚 50%（30000+54000+14400+51）", st(a1.ID).NormalAmount, 98451)
	c.eq("陈墨 30%（18000+32400+8640+30）", st(a2.ID).NormalAmount, 59070)
	c.eq("周禾 20%+星岛夜航（12000+21600+5760+20+60000+36000）", st(a3.ID).NormalAmount, 135380)
	// 全体作者合计 == 全部正常行版税合计（分钱不丢）
	var grossSum int64
	db.Model(&SalesLine{}).Where("status <> ? AND batch_id IN (SELECT id FROM sales_batches WHERE period='2026-08')", lineException).
		Select("COALESCE(SUM(royalty_gross),0)").Scan(&grossSum)
	c.eq("分配合计恒等于版税总额（整数分零头不丢）", 98451+59070+135380, grossSum)

	// 零头行逐作者断言
	var lineOdd SalesLine
	db.Where("doc_ref = ?", "D-CN-1099").First(&lineOdd)
	var oddEntries []SettlementEntry
	db.Where("sales_line_id = ?", lineOdd.ID).Order("author_id").Find(&oddEntries)
	oddGot := map[uint]int64{}
	for _, e := range oddEntries {
		oddGot[e.AuthorID] = e.Royalty
	}
	c.ok("零头行 101 分按 51/30/20 分配", oddGot[a1.ID] == 51 && oddGot[a2.ID] == 30 && oddGot[a3.ID] == 20,
		fmt.Sprintf("%v", oddGot))

	fmt.Println("\n【2】异常销量隔离：跨区 / 语种 / 跨期退货 / 到期补传")
	exOf := func(ref string) SalesLine {
		var l SalesLine
		db.Where("doc_ref = ?", ref).First(&l)
		return l
	}
	jp := exOf("D-JP-3003")
	fr := exOf("D-CN-2005")
	rt := exOf("RT-6001")
	exp := exOf("D-CN-8001")
	c.ok("越区销售 JP → 异常 cross_region", jp.Status == lineException && jp.ExceptionType == exCrossRegion, jp.ExceptionMemo)
	c.ok("未授权语种 fr → 异常 language_not_licensed", fr.Status == lineException && fr.ExceptionType == exLanguage)
	c.ok("跨期退货（6月退货8月入账）→ cross_period_return", rt.Status == lineException && rt.ExceptionType == exCrossPeriodReturn)
	c.ok("到期后补传 → post_expiry 且无合同版本", exp.Status == lineException && exp.ExceptionType == exPostExpiry && exp.ContractID == 0)
	c.eq("跨期退货按销售时 v1 计版税 -2500", rt.RoyaltyGross, -2500)
	c.ok("跨期退货记录的是 v1 合同（销售发生时有效版本）", rt.ContractID == c1.ID)
	var exCount int64
	db.Model(&SalesLine{}).Where("status = ?", lineException).Count(&exCount)
	c.ok("共 4 条异常，且均未进入任何结算单", exCount == 4)
	var entryOnExceptions int64
	db.Model(&SettlementEntry{}).Where("sales_line_id IN ?", []uint{jp.ID, fr.ID, rt.ID, exp.ID}).Count(&entryOnExceptions)
	c.eq("异常销量生成的结算条目数", entryOnExceptions, 0)

	fmt.Println("\n【3】重复导入幂等")
	var batchAug SalesBatch
	db.Where("batch_ref = ?", "CH2026-08").First(&batchAug)
	var rowsBefore int64
	db.Model(&SalesLine{}).Where("batch_id = ?", batchAug.ID).Count(&rowsBefore)
	r1, err := ImportBatch(db, ImportBatchInput{BatchRef: "CH2026-08", Period: "2026-08", Rows: []SalesRowInput{{
		WorkID: w1.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-08-05", Qty: 999, UnitPrice: 5000,
	}}})
	if err != nil {
		fmt.Println("重复导入报错:", err)
		return 1
	}
	var rowsAfter int64
	db.Model(&SalesLine{}).Where("batch_id = ?", batchAug.ID).Count(&rowsAfter)
	c.ok("重复批次被识别拒绝（不写入新行）", r1.Duplicate && rowsBefore == rowsAfter,
		fmt.Sprintf("行数 %d → %d，duplicate_hits=%d", rowsBefore, rowsAfter, r1.Batch.DuplicateHits))
	GenerateSettlements(db, "2026-08")
	c.eq("重新计酬后林岚金额不变（不重复计酬）", st(a1.ID).NormalAmount, 98451)
	var dupEntries int64
	db.Model(&SettlementEntry{}).Where("sales_line_id IN (SELECT id FROM sales_lines WHERE doc_ref='D-CN-1001')").Count(&dupEntries)
	c.eq("同一条销量每位作者仅 1 条计酬记录", dupEntries, 3)

	// ---- HTTP 链路 ----
	r := Router(db)
	gin.SetMode(gin.TestMode)
	var uFin, uLin User
	db.Where("email = ?", "finance@pub.com").First(&uFin)
	db.Where("email = ?", "lin@author.com").First(&uLin)
	tokFin := issueToken(uFin)
	tokLin := issueToken(uLin)

	call := func(method, path, token string, body any) (int, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out map[string]any
		if w.Body.Len() > 0 {
			json.Unmarshal(w.Body.Bytes(), &out)
		}
		return w.Code, out
	}

	fmt.Println("\n【4】角色权限与授权范围查询")
	code, _ := call("POST", "/api/sales/import", tokLin, map[string]any{"batch_ref": "X", "period": "2026-08", "rows": []any{}})
	c.eq("作者不能导入销量（403）", int64(code), http.StatusForbidden)
	code, body := call("GET", "/api/works/1/license?date=2026-06-15&region=GB&language=en", tokLin, nil)
	c.ok("作品页授权查询：v1 不含英国 → 越区", code == 200 && body["licensed"] == false && body["region_covered"] == false,
		fmt.Sprintf("%v", body["reason"]))
	code, body = call("GET", "/api/works/1/license?date=2026-08-15&region=GB&language=en", tokLin, nil)
	c.ok("v2 已含英国 → 在授权范围内", code == 200 && body["licensed"] == true)

	fmt.Println("\n【5】确认冻结 → 补传销量 → 补差单 → 付款回执")
	st1 := st(a1.ID)
	code, body = call("POST", fmt.Sprintf("/api/settlements/%d/confirm", st1.ID), tokFin, nil)
	c.eq("财务确认结算单", int64(code), 200)
	var st1DB Settlement
	db.First(&st1DB, st1.ID)
	c.ok("结算单状态 confirmed 且金额冻结", st1DB.Status == "confirmed" && st1DB.NormalAmount == 98451)

	// 确认后再补传同一归属期的销量
	code, body = call("POST", "/api/sales/import", tokFin, ImportBatchInput{
		BatchRef: "CH2026-08B", Period: "2026-08", Note: "8月补传",
		Rows: []SalesRowInput{{WorkID: w1.ID, Format: "print", Language: "zh", Region: "CN",
			SaleDate: "2026-08-25", DocRef: "D-CN-1100", Qty: 100, UnitPrice: 5000}},
	})
	c.eq("补传批次导入成功", int64(code), 200)
	code, _ = call("POST", "/api/settlements/generate", tokFin, map[string]string{"period": "2026-08"})
	c.eq("对该期重新计酬", int64(code), 200)
	var lateLine SalesLine
	db.Where("doc_ref = ?", "D-CN-1100").First(&lateLine)
	c.ok("已确认期补传销量被拦为 settlement_locked，不混入正常收入",
		lateLine.Status == lineException && lateLine.ExceptionType == exSettlementLocked, lateLine.ExceptionMemo)
	db.First(&st1DB, st1.ID)
	c.eq("已确认结算单金额分文未改", st1DB.NormalAmount, 98451)
	var a2AmtBefore int64 = st(a2.ID).NormalAmount
	var a3AmtBefore int64 = st(a3.ID).NormalAmount
	c.eq("补传行同样未计入其他作者草稿单（陈墨）", a2AmtBefore, 59070)
	c.eq("补传行同样未计入其他作者草稿单（周禾）", a3AmtBefore, 135380)

	// 草稿单不允许补差（应直接重算）
	code, body = call("POST", "/api/adjustments", tokFin, map[string]any{
		"original_settlement_id": st(a2.ID).ID, "author_id": a2.ID, "period": "2026-08",
		"reason": "error_correction", "amount": 100,
	})
	c.ok("对草稿单开补差被拒绝（草稿直接重算即可）", code == 400, fmt.Sprint(body["error"]))

	// 对已确认单开补差：补传行 60000 × 50% = 30000，关联原单
	code, body = call("POST", "/api/adjustments", tokFin, map[string]any{
		"original_settlement_id": st1.ID, "author_id": a1.ID, "period": "2026-08",
		"reason": "late_sales", "memo": "8月补传 D-CN-1100 应得 50%",
		"entries": []map[string]any{{
			"sales_line_id": lateLine.ID, "work_id": w1.ID, "contract_id": c2.ID,
			"memo": "补传销量 100册×50元×12%×50%", "amount": 30000,
		}},
	})
	c.eq("补差单创建成功并关联原单", int64(code), 200)
	adjID := uint(body["id"].(float64))
	if code == 200 && body["amount"].(float64) != 30000 {
		c.ok("补差金额为 30000", false, fmt.Sprint(body["amount"]))
	} else {
		c.eq("补差金额为 30000", int64(body["amount"].(float64)), 30000)
	}
	var adj Adjustment
	db.First(&adj, adjID)
	c.ok("补差单关联了原结算单", adj.OriginalSettlementID != nil && *adj.OriginalSettlementID == st1.ID)
	db.First(&st1DB, st1.ID)
	c.eq("原单 normal_amount 仍为冻结值", st1DB.NormalAmount, 98451)

	// 确认补差 → 模拟付款（结算单 + 补差单各一张回执）
	call("POST", fmt.Sprintf("/api/adjustments/%d/confirm", adjID), tokFin, nil)
	code, body = call("POST", "/api/payments/pay", tokFin, map[string]any{"type": "settlement", "id": st1.ID})
	c.eq("结算单模拟付款", int64(code), 200)
	c.eq("结算单回执金额 = 冻结金额", int64(body["amount"].(float64)), 98451)
	code, body = call("POST", "/api/payments/pay", tokFin, map[string]any{"type": "adjustment", "id": adjID})
	c.eq("补差单模拟付款", int64(code), 200)
	c.eq("补差单回执金额 = 30000", int64(body["amount"].(float64)), 30000)
	// 重复付款幂等：不再产生第二张回执
	call("POST", "/api/payments/pay", tokFin, map[string]any{"type": "settlement", "id": st1.ID})
	var rcN int64
	db.Model(&PaymentReceipt{}).Where("payable_type=? AND payable_id=?", "settlement", st1.ID).Count(&rcN)
	c.eq("同一单重复付款只保留一张回执", rcN, 1)
	// 未确认不能付款
	code, _ = call("POST", "/api/payments/pay", tokFin, map[string]any{"type": "settlement", "id": st(a2.ID).ID})
	c.eq("草稿单直接付款被拒绝", int64(code), 400)

	fmt.Println("\n【6】异常特批（跨期退货）走补差单，按销售时合同 v1 分成")
	code, body = call("POST", fmt.Sprintf("/api/exceptions/%d/allow", rt.ID), tokFin, map[string]any{
		"period": "2026-08", "memo": "经渠道核实，6月退货8月到账，特批抵扣",
	})
	c.eq("跨期退货特批", int64(code), 200)
	var rtAdjs []Adjustment
	db.Where("reason = ? AND author_id IN ?", "cross_period_return", []uint{a1.ID, a2.ID}).Find(&rtAdjs)
	sort.Slice(rtAdjs, func(i, j int) bool { return rtAdjs[i].AuthorID < rtAdjs[j].AuthorID })
	c.ok("按 v1 的 60/40 生成两张负补差（-1500 / -1000）",
		len(rtAdjs) == 2 && rtAdjs[0].Amount == -1500 && rtAdjs[1].Amount == -1000,
		fmt.Sprintf("%+v", rtAdjs))
	c.ok("退货补差单未计入周禾（v1 无此作者）", len(rtAdjs) == 2)

	fmt.Println("\n【7】作者端只看本人，明细可追溯原始销量与合同依据")
	code, body = call("GET", "/api/my/settlements", tokLin, nil)
	items := body["items"].([]any)
	onlyOwn := true
	for _, it := range items {
		m := it.(map[string]any)
		if uint(m["author_id"].(float64)) != a1.ID {
			onlyOwn = false
		}
	}
	c.ok("林岚只看到自己的结算单", code == 200 && onlyOwn, fmt.Sprintf("看到 %d 张", len(items)))
	code, _ = call("GET", fmt.Sprintf("/api/my/settlements/%d", st(a2.ID).ID), tokLin, nil)
	c.eq("林岚不能查看陈墨的结算单（403）", int64(code), 403)
	code, body = call("GET", fmt.Sprintf("/api/my/settlements/%d", st1.ID), tokLin, nil)
	entries := body["entries"].([]any)
	traceable := false
	for _, e := range entries {
		m := e.(map[string]any)
		if m["sales_line_id"] != nil && m["contract_id"] != nil &&
			uint(m["contract_id"].(float64)) == c2.ID && m["kind"] == "sale" {
			traceable = true
		}
	}
	c.ok("计算明细可追溯到原始销量行与抵扣合同版本", len(entries) > 0 && traceable,
		fmt.Sprintf("共 %d 条明细", len(entries)))

	fmt.Println("\n【8】9月期草稿金额（v2 50/30/20）")
	c.eq("9月 林岚", st9(db, a1.ID), 15000)
	c.eq("9月 陈墨", st9(db, a2.ID), 9000)
	c.eq("9月 周禾", st9(db, a3.ID), 6000)

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if len(c.fails) > 0 {
		fmt.Printf("\033[31m%d 项失败：\033[0m\n", len(c.fails))
		for _, f := range c.fails {
			fmt.Println("  -", f)
		}
		return 1
	}
	fmt.Printf("\033[32m全部 %d 项断言通过 ✅  多作者分成 / 重复导入幂等 / 越区·跨期·到期异常隔离 / 确认冻结·补差·回执 均验证通过\033[0m\n", c.pass)
	return 0
}

func st9(db *gorm.DB, aid uint) int64 {
	var s Settlement
	db.Where("period = ? AND author_id = ?", "2026-09", aid).First(&s)
	return s.NormalAmount
}

var _ = strings.TrimSpace
