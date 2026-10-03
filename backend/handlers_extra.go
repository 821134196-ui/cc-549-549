package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---------- 确认（冻结） ----------

func confirmSettlement(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var st Settlement
		if err := db.First(&st, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "结算单不存在"})
			return
		}
		if st.Status != "draft" {
			c.JSON(400, gin.H{"error": "仅草稿结算单可确认，当前状态：" + st.Status})
			return
		}
		// 再生成一次，确保无遗漏条目，然后冻结
		if _, err := GenerateSettlements(db, st.Period); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		db.First(&st, id)
		now := time.Now()
		db.Model(&st).Updates(map[string]any{"status": "confirmed", "confirmed_at": now})
		db.First(&st, id)
		c.JSON(200, loadSettlementDTO(db, st))
	}
}

func confirmAdjustment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var a Adjustment
		if err := db.First(&a, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "补差单不存在"})
			return
		}
		if a.Status != "draft" {
			c.JSON(400, gin.H{"error": "仅草稿补差单可确认，当前状态：" + a.Status})
			return
		}
		now := time.Now()
		db.Model(&a).Updates(map[string]any{"status": "confirmed", "confirmed_at": now})
		db.Preload("Author").Preload("Receipt").First(&a, a.ID)
		db.Order("id").Find(&a.Entries, "adjustment_id = ?", a.ID)
		c.JSON(200, a)
	}
}

// ---------- 财务总览 ----------

func overview(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		type row struct {
			Period string `json:"period"`
			Count  int64  `json:"count"`
			Amount int64  `json:"amount"`
		}
		var rows []row
		db.Model(&Settlement{}).Select("period, COUNT(*) as count, COALESCE(SUM(normal_amount),0) as amount").
			Group("period").Order("period DESC").Scan(&rows)

		var draftN, confN, paidN int64
		var draftAmt, confAmt, paidAmt int64
		db.Model(&Settlement{}).Where("status = ?", "draft").Count(&draftN)
		db.Model(&Settlement{}).Where("status = ?", "confirmed").Count(&confN)
		db.Model(&Settlement{}).Where("status = ?", "paid").Count(&paidN)
		db.Model(&Settlement{}).Where("status = ?", "draft").Select("COALESCE(SUM(normal_amount),0)").Scan(&draftAmt)
		db.Model(&Settlement{}).Where("status = ?", "confirmed").Select("COALESCE(SUM(normal_amount),0)").Scan(&confAmt)
		db.Model(&Settlement{}).Where("status = ?", "paid").Select("COALESCE(SUM(normal_amount),0)").Scan(&paidAmt)

		var exOpen, exResolved int64
		db.Model(&SalesLine{}).Where("status = ? AND resolved = ?", lineException, false).Count(&exOpen)
		db.Model(&SalesLine{}).Where("status = ? AND resolved = ?", lineException, true).Count(&exResolved)

		var adjPending, adjPaid int64
		var adjPendingAmt, adjPaidAmt int64
		db.Model(&Adjustment{}).Where("status <> ?", "paid").Count(&adjPending)
		db.Model(&Adjustment{}).Where("status = ?", "paid").Count(&adjPaid)
		db.Model(&Adjustment{}).Where("status <> ?", "paid").Select("COALESCE(SUM(amount),0)").Scan(&adjPendingAmt)
		db.Model(&Adjustment{}).Where("status = ?", "paid").Select("COALESCE(SUM(amount),0)").Scan(&adjPaidAmt)

		var batches, dupBatches int64
		db.Model(&SalesBatch{}).Count(&batches)
		db.Model(&SalesBatch{}).Where("duplicate_hits > 0").Count(&dupBatches)

		c.JSON(200, gin.H{
			"by_period": rows,
			"settlements": gin.H{
				"draft":     gin.H{"count": draftN, "amount": draftAmt},
				"confirmed": gin.H{"count": confN, "amount": confAmt},
				"paid":      gin.H{"count": paidN, "amount": paidAmt},
			},
			"exceptions": gin.H{"open": exOpen, "resolved": exResolved},
			"adjustments": gin.H{
				"pending": gin.H{"count": adjPending, "amount": adjPendingAmt},
				"paid":    gin.H{"count": adjPaid, "amount": adjPaidAmt},
			},
			"batches": gin.H{"total": batches, "duplicate_rejected": dupBatches},
		})
	}
}

// ---------- 作者端 ----------

func myProfile(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		a := mustAuth(c)
		if a.User.AuthorID == nil {
			c.JSON(400, gin.H{"error": "账号未关联作者"})
			return
		}
		var author Author
		db.First(&author, *a.User.AuthorID)
		c.JSON(200, author)
	}
}

func mySummary(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		a := mustAuth(c)
		aid := *a.User.AuthorID
		var total, paid, confirmed, draft int64
		db.Model(&Settlement{}).Where("author_id = ?", aid).Select("COALESCE(SUM(normal_amount),0)").Scan(&total)
		for _, st := range []struct {
			s string
			v *int64
		}{{"paid", &paid}, {"confirmed", &confirmed}, {"draft", &draft}} {
			db.Model(&Settlement{}).Where("author_id = ? AND status = ?", aid, st.s).
				Select("COALESCE(SUM(normal_amount),0)").Scan(st.v)
		}
		var adjTotal, adjPaid int64
		db.Model(&Adjustment{}).Where("author_id = ?", aid).Select("COALESCE(SUM(amount),0)").Scan(&adjTotal)
		db.Model(&Adjustment{}).Where("author_id = ? AND status = ?", aid, "paid").
			Select("COALESCE(SUM(amount),0)").Scan(&adjPaid)
		c.JSON(200, gin.H{
			"normal_total": total, "draft": draft, "confirmed": confirmed, "paid": paid,
			"adjustment_total": adjTotal, "adjustment_paid": adjPaid,
			"all_time_payable": paid + adjPaid,
		})
	}
}

func mySettlements(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		a := mustAuth(c)
		aid := *a.User.AuthorID
		var sts []Settlement
		q := db.Where("author_id = ?", aid)
		if p := c.Query("period"); p != "" {
			q = q.Where("period = ?", p)
		}
		q.Order("period DESC, id").Find(&sts)
		out := []settlementDTO{}
		for _, st := range sts {
			out = append(out, loadSettlementDTO(db, st))
		}
		c.JSON(200, gin.H{"items": out})
	}
}

func mySettlementDetail(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		a := mustAuth(c)
		id, _ := strconv.Atoi(c.Param("id"))
		var st Settlement
		if err := db.First(&st, id).Error; err != nil {
			c.JSON(404, gin.H{"error": "结算单不存在"})
			return
		}
		if st.AuthorID != *a.User.AuthorID {
			c.JSON(http.StatusForbidden, gin.H{"error": "只能查看本人结算单"})
			return
		}
		c.JSON(200, loadSettlementDTO(db, st))
	}
}

func myAdjustments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		a := mustAuth(c)
		aid := *a.User.AuthorID
		var adjs []Adjustment
		db.Where("author_id = ?", aid).Order("id DESC").Find(&adjs)
		out := make([]Adjustment, 0, len(adjs))
		for _, adj := range adjs {
			db.Preload("Receipt").First(&adj, adj.ID)
			db.Order("id").Find(&adj.Entries, "adjustment_id = ?", adj.ID)
			out = append(out, adj)
		}
		c.JSON(200, gin.H{"items": out})
	}
}
