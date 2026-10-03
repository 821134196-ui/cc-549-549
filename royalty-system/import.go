package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// parseYuan 把 "5000.00" 这样的元字符串精确转为分
func parseYuan(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("金额为空")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("非法金额 %q", s)
	}
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("非法金额 %q", s)
	}
	cents := yuan * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		} else if len(frac) > 2 {
			frac = frac[:2]
		}
		v, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("非法金额 %q", s)
		}
		cents += v
	}
	if neg {
		cents = -cents
	}
	return cents, nil
}

type ImportResult struct {
	BatchID      uint     `json:"batch_id"`
	Period       string   `json:"period"`
	ImportedRows int      `json:"imported_rows"`
	SkippedDup   int      `json:"skipped_duplicate_refs"`
	Errors       []string `json:"errors"`
}

// ImportSales 解析并导入销量 CSV。批次内容哈希唯一 → 同一批文件重复导入直接拒绝；
// 行级 ref 唯一索引 → 即使内容不同也不会对同一渠道行重复计酬。
// 表头：ref,work_code,sale_date,period,region,language,format,qty,unit_price,amount,original_ref
func ImportSales(db *gorm.DB, fileName string, r io.Reader) (*ImportResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	var existing SalesBatch
	if err := db.Where("ref_hash = ?", hash).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("该文件已导入（批次 #%d，期号 %s），重复导入被拒绝以防止重复计酬", existing.ID, existing.Period)
	}

	cr := csv.NewReader(strings.NewReader(string(data)))
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	if len(rows) < 2 {
		return nil, errors.New("CSV 没有数据行")
	}

	workCache := map[string]uint{}
	period := ""
	var records []SalesRecord
	res := &ImportResult{Errors: []string{}}
	for i, row := range rows[1:] {
		lineNo := i + 2
		if len(row) < 10 {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行列数不足，跳过", lineNo))
			continue
		}
		ref := strings.TrimSpace(row[0])
		workCode := strings.TrimSpace(row[1])
		wid, ok := workCache[workCode]
		if !ok {
			var w Work
			if err := db.Where("code = ?", workCode).First(&w).Error; err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行作品编号 %s 不存在，跳过", lineNo, workCode))
				continue
			}
			wid = w.ID
			workCache[workCode] = wid
		}
		saleDate, derr := time.ParseInLocation("2006-01-02", strings.TrimSpace(row[2]), time.Local)
		if derr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行日期格式错误（应为 YYYY-MM-DD），跳过", lineNo))
			continue
		}
		rowPeriod := strings.TrimSpace(row[3])
		if rowPeriod == "" {
			rowPeriod = periodOf(saleDate)
		}
		if period == "" {
			period = rowPeriod
		}
		qty, qerr := strconv.ParseInt(strings.TrimSpace(row[7]), 10, 64)
		if qerr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行数量格式错误，跳过", lineNo))
			continue
		}
		unit, _ := parseYuan(row[8])
		amount, aerr := parseYuan(row[9])
		if aerr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行 %v，跳过", lineNo, aerr))
			continue
		}
		format := strings.TrimSpace(row[6])
		if format != "print" && format != "ebook" {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行格式须为 print/ebook，跳过", lineNo))
			continue
		}
		rec := SalesRecord{
			BatchID:        0,
			WorkID:         wid,
			Ref:            ref,
			OriginalRef:    strings.TrimSpace(row[10]),
			SaleDate:       saleDate,
			Period:         rowPeriod,
			Region:         strings.TrimSpace(row[4]),
			Language:       strings.TrimSpace(row[5]),
			Format:         format,
			Qty:            qty,
			UnitPriceCents: unit,
			AmountCents:    amount,
		}
		records = append(records, rec)
	}
	if period == "" {
		return nil, errors.New("没有可导入的有效行")
	}

	batch := SalesBatch{Period: period, RefHash: hash, FileName: fileName, RowCount: len(records), ImportedAt: time.Now()}
	if err := db.Create(&batch).Error; err != nil {
		return nil, fmt.Errorf("创建批次失败: %w", err)
	}
	for i := range records {
		records[i].BatchID = batch.ID
		// 关联退货抵扣的原销售
		if records[i].Qty < 0 && records[i].OriginalRef != "" {
			var orig SalesRecord
			if err := db.Where("ref = ? AND work_id = ?", records[i].OriginalRef, records[i].WorkID).First(&orig).Error; err == nil {
				records[i].DeductRecordID = &orig.ID
			}
		}
		if err := db.Create(&records[i]).Error; err != nil {
			if isUniqueErr(err) {
				res.SkippedDup++
				continue
			}
			res.Errors = append(res.Errors, fmt.Sprintf("行 %s 写入失败: %v", records[i].Ref, err))
			continue
		}
		res.ImportedRows++
	}
	res.BatchID = batch.ID
	res.Period = period
	return res, nil
}

func isUniqueErr(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint")
}
