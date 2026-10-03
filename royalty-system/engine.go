package main

import (
	"sort"
	"time"

	"gorm.io/gorm"
)

// periodOf 返回 YYYY-MM
func periodOf(t time.Time) string {
	return t.Format("2006-01")
}

// effectiveContract 返回某日生效的合同版本（effective_from <= date < effective_until）
func effectiveContract(db *gorm.DB, workID uint, date time.Time) (*ContractVersion, error) {
	var v ContractVersion
	err := db.Where("work_id = ? AND effective_from <= ? AND (effective_until IS NULL OR effective_until > ?)",
		workID, date, date).Order("effective_from DESC, id DESC").First(&v).Error
	if err != nil {
		return nil, err
	}
	if err := loadContractDetail(db, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// latestContract 返回作品最近创建的合同版本（异常销量无法确定合同时的作者归属兜底）
func latestContract(db *gorm.DB, workID uint) (*ContractVersion, error) {
	var v ContractVersion
	err := db.Where("work_id = ?", workID).Order("effective_from DESC, id DESC").First(&v).Error
	if err != nil {
		return nil, err
	}
	if err := loadContractDetail(db, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func loadContractDetail(db *gorm.DB, v *ContractVersion) error {
	if err := db.Where("contract_version_id = ?", v.ID).Find(&v.Participants).Error; err != nil {
		return err
	}
	if err := db.Where("contract_version_id = ?", v.ID).Find(&v.Regions).Error; err != nil {
		return err
	}
	return db.Where("contract_version_id = ?", v.ID).Find(&v.Languages).Error
}

type participantShare struct {
	ContractParticipant
}

type linePlan struct {
	record   SalesRecord
	contract *ContractVersion // 用于计算的合同（异常时可能为兜底合同）
	category string
	reason   string
	shares   []struct {
		authorID uint
		shareBPS int
		amount   int64
	}
	rateBPS int
}

// classifyRecord 判定销量分类并计算各作者分成金额（整数分，最大余数法，合计无尾差）
func classifyRecord(db *gorm.DB, r SalesRecord) (*linePlan, error) {
	plan := &linePlan{record: r}

	c, err := effectiveContract(db, r.WorkID, r.SaleDate)
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
		// 销售发生时无有效合同（合同到期后补传的销量）
		fallback, ferr := latestContract(db, r.WorkID)
		if ferr != nil {
			// 作品从未签过合同：无法归属作者，记录级异常保留
			plan.category = CatNoContract
			plan.reason = "销售发生时无有效合同，且作品无任何合同版本，无法归属作者"
			return plan, nil
		}
		plan.contract = fallback
		plan.category = CatNoContract
		plan.reason = "销售发生时无有效合同（可能为合同到期后补传），按最近合同版本估算仅供核对"
	} else {
		plan.contract = c
		// 授权区域 / 语种检查
		regionOK, langOK := false, false
		for _, rg := range c.Regions {
			if rg.Region == r.Region {
				regionOK = true
				break
			}
		}
		for _, lg := range c.Languages {
			if lg.Language == r.Language {
				langOK = true
				break
			}
		}
		if !regionOK || !langOK {
			plan.category = CatCrossRegion
			if !regionOK && !langOK {
				plan.reason = "销售区域 " + r.Region + " 与语种 " + r.Language + " 均不在授权范围内"
			} else if !regionOK {
				plan.reason = "销售区域 " + r.Region + " 不在授权区域内（跨区销售）"
			} else {
				plan.reason = "销售语种 " + r.Language + " 不在授权语种内"
			}
		}
	}

	// 退货抵扣检查（qty 为负）
	if r.Qty < 0 {
		var original SalesRecord
		q := db.Where("work_id = ? AND region = ? AND language = ? AND format = ? AND qty > 0",
			r.WorkID, r.Region, r.Language, r.Format)
		if r.OriginalRef != "" {
			q = q.Where("ref = ?", r.OriginalRef)
		} else {
			q = q.Where("sale_date <= ?", r.SaleDate).Order("sale_date DESC, id DESC")
		}
		oerr := q.First(&original).Error
		if oerr == nil {
			if periodOf(original.SaleDate) != r.Period {
				plan.category = CatCrossPeriodReturn
				plan.reason = "退货抵扣的原销售发生在 " + periodOf(original.SaleDate) + " 期，属于跨期退货（原单 " + original.Ref + "）"
			} else if plan.category == CatNormal {
				plan.reason = "同期退货，抵扣原销售 " + original.Ref
			}
		} else if plan.category == CatNormal {
			plan.category = CatCrossPeriodReturn
			plan.reason = "未找到可抵扣的原销售记录，按跨期退货隔离处理"
		}
	}

	if plan.category == "" {
		plan.category = CatNormal
	}

	rate := plan.contract.PrintRateBPS
	if r.Format == "ebook" {
		rate = plan.contract.EbookRateBPS
	}
	plan.rateBPS = rate

	// 计算作者分成：最大余数法保证合计精确
	parts := plan.contract.Participants
	sort.Slice(parts, func(i, j int) bool { return parts[i].ID < parts[j].ID })

	total := r.AmountCents * int64(rate)
	target := total / 10000
	if total%10000 >= 5000 { // 四舍五入到分
		target++
	}
	type rem struct {
		idx int
		rem int64
	}
	floors := make([]int64, len(parts))
	rems := make([]rem, len(parts))
	used := int64(0)
	for i, p := range parts {
		v := target * int64(p.ShareBPS)
		floors[i] = v / 10000
		rems[i] = rem{i, v % 10000}
		used += floors[i]
	}
	left := target - used
	sort.SliceStable(rems, func(i, j int) bool {
		if rems[i].rem != rems[j].rem {
			return rems[i].rem > rems[j].rem
		}
		return parts[rems[i].idx].ShareBPS > parts[rems[j].idx].ShareBPS
	})
	for k := int64(0); k < left; k++ {
		floors[rems[int(k)%len(rems)].idx]++
	}
	for i, p := range parts {
		plan.shares = append(plan.shares, struct {
			authorID uint
			shareBPS int
			amount   int64
		}{p.AuthorID, p.ShareBPS, floors[i]})
	}
	return plan, nil
}

// RegenerateSettlements 删除所有草稿结算单，依据销量重新生成。
// 覆盖判定精确到（销量记录 × 作者）：某作者已确认/已付款结算单中的行保留，
// 该记录对其他作者仍可进入草稿结算；对同一作者的新销量进入关联原单的补结算单（supplement）。
func RegenerateSettlements(db *gorm.DB) (records, lines int, err error) {
	var draftIDs []uint
	if err = db.Model(&Settlement{}).Where("status = ?", StatusDraft).Pluck("id", &draftIDs).Error; err != nil {
		return 0, 0, err
	}
	if len(draftIDs) > 0 {
		if err = db.Where("settlement_id IN ?", draftIDs).Delete(&SettlementLine{}).Error; err != nil {
			return 0, 0, err
		}
		if err = db.Where("id IN ?", draftIDs).Delete(&Settlement{}).Error; err != nil {
			return 0, 0, err
		}
	}

	// 已锁定（确认/付款）的行：(sales_record_id, author_id) 不允许再次计酬
	type ra struct {
		R uint
		A uint
	}
	locked := map[ra]bool{}
	var lockedLines []SettlementLine
	if err = db.Where("settlement_id IN (SELECT id FROM settlements WHERE status IN ?)",
		[]string{StatusConfirmed, StatusPaid}).Find(&lockedLines).Error; err != nil {
		return 0, 0, err
	}
	for _, ln := range lockedLines {
		locked[ra{ln.SalesRecordID, ln.AuthorID}] = true
	}

	// 已确认的正常结算单索引：(author,period) -> settlement
	confirmed := map[string]*Settlement{}
	var confs []Settlement
	if err = db.Where("status IN ? AND kind = ?", []string{StatusConfirmed, StatusPaid}, KindNormal).Find(&confs).Error; err != nil {
		return 0, 0, err
	}
	for i := range confs {
		confirmed[keyAP(confs[i].AuthorID, confs[i].Period)] = &confs[i]
	}

	var allRecords []SalesRecord
	if err = db.Order("sale_date, id").Find(&allRecords).Error; err != nil {
		return 0, 0, err
	}

	type settKey struct {
		authorID uint
		period   string
		kind     string
		linked   uint // 0 表示无
	}
	ents := map[settKey]*Settlement{}
	type lineWithKey struct {
		line SettlementLine
		key  settKey
	}
	var pending []lineWithKey

	getOrCreate := func(k settKey) *Settlement {
		if e, ok := ents[k]; ok {
			return e
		}
		s := &Settlement{AuthorID: k.authorID, Period: k.period, Kind: k.kind, Status: StatusDraft}
		if k.linked != 0 {
			id := k.linked
			s.LinkedSettlementID = &id
			s.Note = "原结算单确认后补录销量形成的补结算单"
		}
		ents[k] = s
		return s
	}

	for _, r := range allRecords {
		records++
		plan, err := classifyRecord(db, r)
		if err != nil {
			return 0, 0, err
		}
		if len(plan.shares) == 0 {
			continue // 无任何合同可归属，仅保留在销量异常清单中
		}
		var cid *uint
		if plan.contract != nil {
			id := plan.contract.ID
			cid = &id
		}
		for _, sh := range plan.shares {
			if locked[ra{r.ID, sh.authorID}] {
				continue // 该作者此销量已在确认单中计酬，不重复计
			}
			k := settKey{authorID: sh.authorID, period: r.Period, kind: KindNormal}
			if conf := confirmed[keyAP(sh.authorID, r.Period)]; conf != nil {
				k.kind = KindSupplement
				k.linked = conf.ID
			}
			st := getOrCreate(k)
			if plan.category == CatNormal {
				st.NormalAmountCents += sh.amount
			} else {
				st.ExceptionAmountCents += sh.amount
			}
			pending = append(pending, lineWithKey{
				key: k,
				line: SettlementLine{
					SalesRecordID:     r.ID,
					AuthorID:          sh.authorID,
					WorkID:            r.WorkID,
					ContractVersionID: cid,
					Category:          plan.category,
					Format:            r.Format,
					SaleDate:          r.SaleDate,
					Region:            r.Region,
					Language:          r.Language,
					BaseAmountCents:   r.AmountCents,
					RateBPS:           plan.rateBPS,
					ShareBPS:          sh.shareBPS,
					AmountCents:       sh.amount,
					Reason:            plan.reason,
				},
			})
		}
	}

	for _, s := range ents {
		if err = db.Create(s).Error; err != nil {
			return 0, 0, err
		}
	}
	for _, p := range pending {
		st := ents[p.key]
		p.line.SettlementID = st.ID
		if err = db.Create(&p.line).Error; err != nil {
			return 0, 0, err
		}
		lines++
	}
	return records, lines, nil
}

func keyAP(authorID uint, period string) string {
	return period + "#" + itoa(int(authorID))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
