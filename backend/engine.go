package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	lineNormal    = "normal"
	lineException = "exception"
	lineSettled   = "settled"

	exCrossRegion       = "cross_region"          // 越区销售
	exCrossPeriodReturn = "cross_period_return"   // 跨期退货
	exPostExpiry        = "post_expiry"           // 合同到期后补传
	exLanguage          = "language_not_licensed" // 授权语种之外
	exNoContract        = "no_contract"           // 销售日无任何有效合同
	exSettlementLocked  = "settlement_locked"     // 归属期结算单已确认，须走补差
)

// SalesRowInput 导入用的一行销量
type SalesRowInput struct {
	WorkID    uint   `json:"work_id"`
	Format    string `json:"format"`
	Language  string `json:"language"`
	Region    string `json:"region"`
	SaleDate  string `json:"sale_date"`
	DocRef    string `json:"doc_ref"`
	ReturnRef string `json:"return_ref"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

// ImportBatchInput 导入请求
type ImportBatchInput struct {
	BatchRef string          `json:"batch_ref"`
	Period   string          `json:"period"`
	Note     string          `json:"note"`
	Rows     []SalesRowInput `json:"rows"`
}

// ImportResult 导入结果
type ImportResult struct {
	Batch      *SalesBatch `json:"batch"`
	Duplicate  bool        `json:"duplicate"`
	Normal     int         `json:"normal"`
	Exceptions int         `json:"exceptions"`
	Royalty    int64       `json:"royalty"` // 正常明细全体作者版税合计
	Reasons    []string    `json:"reasons"`
}

func inList(csv, code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, p := range strings.Split(csv, ",") {
		if strings.ToUpper(strings.TrimSpace(p)) == code {
			return true
		}
	}
	return false
}

func periodOf(date string) string {
	if len(date) >= 7 {
		return date[:7]
	}
	return ""
}

// findContractAt 返回销售发生日有效的合同版本（EffectiveFrom <= d < EffectiveTo）
func findContractAt(db *gorm.DB, workID uint, date string) (ContractVersion, []ContractShare, bool) {
	var cv ContractVersion
	err := db.Where("work_id = ? AND effective_from <= ? AND effective_to > ?", workID, date, date).
		Order("version_no DESC").First(&cv).Error
	if err != nil {
		return cv, nil, false
	}
	var shares []ContractShare
	db.Where("contract_id = ?", cv.ID).Find(&shares)
	sort.Slice(shares, func(i, j int) bool { return shares[i].ID < shares[j].ID })
	return cv, shares, true
}

// latestContract 作品最近一个合同版本（用于异常销量特批时指定计酬依据）
func latestContract(db *gorm.DB, workID uint) (ContractVersion, []ContractShare, bool) {
	var cv ContractVersion
	if err := db.Where("work_id = ?", workID).Order("version_no DESC").First(&cv).Error; err != nil {
		return cv, nil, false
	}
	var shares []ContractShare
	db.Where("contract_id = ?", cv.ID).Find(&shares)
	sort.Slice(shares, func(i, j int) bool { return shares[i].ID < shares[j].ID })
	return cv, shares, true
}

// calcGross 按合同费率计算该销量的版税总额（全体作者），退货自然为负
func calcGross(line *SalesLine, cv ContractVersion) int64 {
	rateType, rate := cv.PrintRateType, cv.PrintRate
	if line.Format == "ebook" {
		rateType, rate = cv.EbookRateType, cv.EbookRate
	}
	if rateType == "per_copy" {
		return line.Qty * rate
	}
	return divBps(line.Amount, rate)
}

// classify 判定一行销量：合同匹配 + 异常分类 + 版税估算
func classify(db *gorm.DB, line *SalesLine, importPeriod string) {
	cv, _, ok := findContractAt(db, line.WorkID, line.SaleDate)
	if !ok {
		// 销售日落在所有合同区间之外：区分"到期后补传"与"签约前"
		var last ContractVersion
		err := db.Where("work_id = ?", line.WorkID).Order("effective_to DESC").First(&last).Error
		line.ContractID = 0
		if err == nil && line.SaleDate >= last.EffectiveTo {
			line.Status, line.ExceptionType = lineException, exPostExpiry
			line.ExceptionMemo = fmt.Sprintf("销售日 %s 已晚于合同最晚到期日 %s", line.SaleDate, last.EffectiveTo)
		} else {
			line.Status, line.ExceptionType = lineException, exNoContract
			line.ExceptionMemo = fmt.Sprintf("销售日 %s 无有效合同版本", line.SaleDate)
		}
		line.RoyaltyGross = 0
		return
	}
	line.ContractID = cv.ID
	line.RoyaltyGross = calcGross(line, cv)

	// 跨期退货：退货发生月与导入归属期不同 → 单独列示
	if line.Qty < 0 && periodOf(line.SaleDate) != importPeriod {
		line.Status, line.ExceptionType = lineException, exCrossPeriodReturn
		ref := line.DocRef
		if line.ReturnRef != "" {
			ref = line.ReturnRef
		}
		line.ExceptionMemo = fmt.Sprintf("退货归属期 %s 与导入期 %s 不一致，原单 %s", periodOf(line.SaleDate), importPeriod, ref)
		return
	}
	if !inList(cv.Regions, line.Region) {
		line.Status, line.ExceptionType = lineException, exCrossRegion
		line.ExceptionMemo = fmt.Sprintf("销售区域 %s 不在合同 v%d 授权区域 %s 内", line.Region, cv.VersionNo, cv.Regions)
		return
	}
	if !inList(cv.Languages, line.Language) {
		line.Status, line.ExceptionType = lineException, exLanguage
		line.ExceptionMemo = fmt.Sprintf("语种 %s 不在合同 v%d 授权语种 %s 内", line.Language, cv.VersionNo, cv.Languages)
		return
	}
	line.Status = lineNormal
}

// ImportBatch 幂等导入：同一 BatchRef 第二次导入直接返回原批次，不产生任何新行
func ImportBatch(db *gorm.DB, in ImportBatchInput) (ImportResult, error) {
	res := ImportResult{Reasons: []string{}}
	if in.BatchRef == "" {
		return res, errors.New("batch_ref 必填")
	}
	if len(in.Period) != 7 {
		return res, errors.New("period 必须为 YYYY-MM")
	}
	if len(in.Rows) == 0 {
		return res, errors.New("rows 不能为空")
	}
	for i, r := range in.Rows {
		if r.Format != "print" && r.Format != "ebook" {
			return res, fmt.Errorf("第 %d 行 format 必须为 print/ebook", i+1)
		}
		if len(r.SaleDate) != 10 {
			return res, fmt.Errorf("第 %d 行 sale_date 必须为 YYYY-MM-DD", i+1)
		}
		var w Work
		if err := db.First(&w, r.WorkID).Error; err != nil {
			return res, fmt.Errorf("第 %d 行作品不存在: %d", i+1, r.WorkID)
		}
	}

	var existing SalesBatch
	if err := db.Where("batch_ref = ?", in.BatchRef).First(&existing).Error; err == nil {
		// 幂等命中：原样返回，不计酬第二次
		db.Preload("Lines").First(&existing, existing.ID)
		db.Model(&existing).UpdateColumn("duplicate_hits", existing.DuplicateHits+1)
		res.Batch = &existing
		res.Duplicate = true
		for _, l := range existing.Lines {
			if l.Status == lineException {
				res.Exceptions++
			} else {
				res.Normal++
				res.Royalty += l.RoyaltyGross
			}
		}
		res.Reasons = append(res.Reasons, fmt.Sprintf("批次 %s 已导入过（%d 行），本次为重复导入，已全部跳过、不重复计酬", in.BatchRef, existing.RowCount))
		return res, nil
	}

	batch := SalesBatch{BatchRef: in.BatchRef, Period: in.Period, Note: in.Note, ImportedAt: time.Now()}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		for _, r := range in.Rows {
			line := SalesLine{
				BatchID: batch.ID, WorkID: r.WorkID, Format: r.Format, Language: strings.ToLower(r.Language),
				Region: strings.ToUpper(r.Region), SaleDate: r.SaleDate, DocRef: r.DocRef, ReturnRef: r.ReturnRef,
				Qty: r.Qty, UnitPrice: r.UnitPrice, Amount: r.Qty * r.UnitPrice,
			}
			classify(tx, &line, in.Period)
			if err := tx.Create(&line).Error; err != nil {
				return err
			}
			if line.Status == lineException {
				res.Exceptions++
				res.Reasons = append(res.Reasons, fmt.Sprintf("异常[%s] 作品%d %s %s %s qty=%d：%s",
					line.ExceptionType, line.WorkID, line.SaleDate, line.Region, line.Format, line.Qty, line.ExceptionMemo))
			} else {
				res.Normal++
				res.Royalty += line.RoyaltyGross
			}
		}
		batch.RowCount = len(in.Rows)
		return tx.Model(&batch).Update("row_count", len(in.Rows)).Error
	})
	if err != nil {
		return res, err
	}
	db.First(&batch, batch.ID)
	res.Batch = &batch
	return res, nil
}

// GenerateSettlements 为某结算期生成/补齐草稿结算单。
// 规则：只取状态 normal 且尚未生成结算条目的销量；每条销量按销售时有效的合同
// 版本给每位作者生成一条条目（(作者,销量) 唯一索引兜底，绝不重复计酬）。
func GenerateSettlements(db *gorm.DB, period string) (map[uint]int64, error) {
	affected := map[uint]int64{}
	var lines []SalesLine
	err := db.Where(`status = ? AND id IN (
		SELECT sl.id FROM sales_lines sl
		JOIN sales_batches sb ON sb.id = sl.batch_id
		WHERE sb.period = ?)`, lineNormal, period).
		Order("id").Find(&lines).Error
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		var n int64
		db.Model(&SettlementEntry{}).Where("sales_line_id = ?", line.ID).Count(&n)
		if n > 0 {
			continue // 已计酬（唯一索引也会兜底）
		}
		var shares []ContractShare
		if err := db.Where("contract_id = ?", line.ContractID).Find(&shares).Error; err != nil {
			return nil, err
		}
		if len(shares) == 0 {
			continue
		}
		sort.Slice(shares, func(i, j int) bool { return shares[i].ID < shares[j].ID })

		// 行级锁检查：只要有一位作者该期结算单已确认/已付款，整条销量都不能再走正常收入，
		// 统一转为异常，由财务开补差单（关联原单）处理，保证确认单金额不被改写。
		var lockedAuthor *string
		for _, s := range shares {
			var st Settlement
			err := db.Where("period = ? AND author_id = ?", period, s.AuthorID).First(&st).Error
			if err == nil && st.Status != "draft" {
				name := s.AuthorID
				_ = name
				lockedAuthor = &st.Status
				break
			}
		}
		if lockedAuthor != nil {
			db.Model(&line).Updates(map[string]any{
				"status": lineException, "exception_type": exSettlementLocked,
				"exception_memo": fmt.Sprintf("%s 期已有结算单确认/付款（状态 %s），该销量须开补差单处理", period, *lockedAuthor),
			})
			continue
		}

		bpsList := make([]int64, len(shares))
		for i, s := range shares {
			bpsList[i] = s.ShareBps
		}
		parts := splitByBps(line.RoyaltyGross, bpsList)

		for i, s := range shares {
			var st Settlement
			err := db.Where("period = ? AND author_id = ?", period, s.AuthorID).First(&st).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				st = Settlement{Period: period, AuthorID: s.AuthorID, Status: "draft"}
				if err := db.Create(&st).Error; err != nil {
					return nil, err
				}
			} else if err != nil {
				return nil, err
			}
			kind := "sale"
			if line.Qty < 0 {
				kind = "return"
			}
			e := SettlementEntry{
				SettlementID: st.ID, AuthorID: s.AuthorID, WorkID: line.WorkID,
				ContractID: line.ContractID, SalesLineID: line.ID, Format: line.Format,
				Region: line.Region, SaleDate: line.SaleDate, Kind: kind, Qty: line.Qty,
				Amount: line.Amount, RoyaltyGross: line.RoyaltyGross,
				ShareBps: s.ShareBps, Royalty: parts[i],
			}
			if err := db.Create(&e).Error; err != nil {
				return nil, err
			}
			affected[s.AuthorID] += parts[i]
		}
	}

	// 重算受影响草稿单合计（normal_amount 恒等于其条目之和，且条目一经确认不可变）
	for authorID := range affected {
		var st Settlement
		if err := db.Where("period = ? AND author_id = ?", period, authorID).First(&st).Error; err != nil {
			continue
		}
		if st.Status != "draft" {
			delete(affected, authorID)
			continue
		}
		var sum int64
		db.Model(&SettlementEntry{}).Where("settlement_id = ?", st.ID).Select("COALESCE(SUM(royalty),0)").Scan(&sum)
		db.Model(&st).Update("normal_amount", sum)
	}
	// 全部正常销量已入账的行标记 settled（仅展示用）
	db.Model(&SalesLine{}).Where("status = ? AND id IN (SELECT sales_line_id FROM settlement_entries)", lineNormal).
		Update("status", lineSettled)
	return affected, nil
}
