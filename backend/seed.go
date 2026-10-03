package main

import "gorm.io/gorm"

// Seed 写入示例数据：
//   - 《山海图注》：两位作者 → 三位作者，两版合同（费率/区域/语种变化，v2 新增英国区域）
//   - 《星岛夜航》：单一作者，纸书按册计酬
//   - 《旧译丛》：合同已到期（用于"到期后补传销量"）
//   - 销量覆盖：正常、越区(JP)、语种未授权(fr)、跨期退货(6月退货8月导入)、
//     到期补传、同批次重复导入
func Seed(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// ---- 用户 ----
		a1 := Author{Name: "林岚", Email: "lin@author.com", BankInfo: "招商银行 6228 **** 1001"}
		a2 := Author{Name: "陈墨", Email: "chen@author.com", BankInfo: "工商银行 6222 **** 2048"}
		a3 := Author{Name: "周禾", Email: "zhou@author.com", BankInfo: "建设银行 6217 **** 3390"}
		if err := tx.Create(&a1).Error; err != nil {
			return err
		}
		if err := tx.Create(&a2).Error; err != nil {
			return err
		}
		if err := tx.Create(&a3).Error; err != nil {
			return err
		}
		createUser(tx, "editor@pub.com", "editor123", "林编辑", "editor", nil)
		createUser(tx, "finance@pub.com", "finance123", "钱财务", "finance", nil)
		createUser(tx, "lin@author.com", "author123", a1.Name, "author", &a1.ID)
		createUser(tx, "chen@author.com", "author123", a2.Name, "author", &a2.ID)
		createUser(tx, "zhou@author.com", "author123", a3.Name, "author", &a3.ID)

		// ---- 作品 ----
		w1 := Work{Title: "山海图注", ISBN: "978-7-00-000001-1", Description: "多作者作品，2026-07 起合同升版：新增英国授权与第三作者，费率上调"}
		w2 := Work{Title: "星岛夜航", ISBN: "978-7-00-000002-8", Description: "单作者作品，纸书按册计酬（2.00 元/册）"}
		w3 := Work{Title: "旧译丛", ISBN: "978-7-00-000003-5", Description: "合同 2026-03-01 到期，用于演示到期后补传销量"}
		for _, w := range []*Work{&w1, &w2, &w3} {
			if err := tx.Create(w).Error; err != nil {
				return err
			}
		}

		// ---- 合同版本 ----
		// 山海图注 v1：2025-01-01 ~ 2026-07-01，林岚60% 陈墨40%，区域 CN,US，纸10% 电15%
		c1 := ContractVersion{
			WorkID: w1.ID, VersionNo: 1, EffectiveFrom: "2025-01-01", EffectiveTo: "2026-07-01",
			Languages: "zh,en", Regions: "CN,US",
			PrintRateType: "percent", PrintRate: 1000, EbookRateType: "percent", EbookRate: 1500,
			Note: "初版合同",
		}
		if err := tx.Create(&c1).Error; err != nil {
			return err
		}
		tx.Create(&ContractShare{ContractID: c1.ID, AuthorID: a1.ID, ShareBps: 6000})
		tx.Create(&ContractShare{ContractID: c1.ID, AuthorID: a2.ID, ShareBps: 4000})

		// 山海图注 v2：2026-07-01 起，林岚50% 陈墨30% 周禾20%，新增 GB，纸12% 电18%
		c2 := ContractVersion{
			WorkID: w1.ID, VersionNo: 2, EffectiveFrom: "2026-07-01", EffectiveTo: "2999-12-31",
			Languages: "zh,en", Regions: "CN,US,GB",
			PrintRateType: "percent", PrintRate: 1200, EbookRateType: "percent", EbookRate: 1800,
			Note: "续约升版：新增英国区域与合著者周禾，费率上调",
		}
		if err := tx.Create(&c2).Error; err != nil {
			return err
		}
		tx.Create(&ContractShare{ContractID: c2.ID, AuthorID: a1.ID, ShareBps: 5000})
		tx.Create(&ContractShare{ContractID: c2.ID, AuthorID: a2.ID, ShareBps: 3000})
		tx.Create(&ContractShare{ContractID: c2.ID, AuthorID: a3.ID, ShareBps: 2000})

		// 星岛夜航：周禾独有，纸书 200 分/册，电子书 12%，仅 CN
		c3 := ContractVersion{
			WorkID: w2.ID, VersionNo: 1, EffectiveFrom: "2026-01-01", EffectiveTo: "2999-12-31",
			Languages: "zh", Regions: "CN",
			PrintRateType: "per_copy", PrintRate: 200, EbookRateType: "percent", EbookRate: 1200,
		}
		if err := tx.Create(&c3).Error; err != nil {
			return err
		}
		tx.Create(&ContractShare{ContractID: c3.ID, AuthorID: a3.ID, ShareBps: 10000})

		// 旧译丛：周禾独有，2026-03-01 到期，纸书 8%，仅 CN
		c4 := ContractVersion{
			WorkID: w3.ID, VersionNo: 1, EffectiveFrom: "2026-01-01", EffectiveTo: "2026-03-01",
			Languages: "zh", Regions: "CN",
			PrintRateType: "percent", PrintRate: 800, EbookRateType: "percent", EbookRate: 800,
			Note: "已到期未续约",
		}
		if err := tx.Create(&c4).Error; err != nil {
			return err
		}
		tx.Create(&ContractShare{ContractID: c4.ID, AuthorID: a3.ID, ShareBps: 10000})

		// ---- 销量：2026-08 期 ----
		batchAug := ImportBatchInput{
			BatchRef: "CH2026-08", Period: "2026-08", Note: "8月渠道对账单",
			Rows: []SalesRowInput{
				// 1) 正常纸书 CN：500000分 × 12% = 60000；50/30/20 → 30000/18000/12000
				{WorkID: w1.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-08-05",
					DocRef: "D-CN-1001", Qty: 100, UnitPrice: 5000},
				// 2) 正常电子书 US：600000 × 18% = 108000 → 54000/32400/21600
				{WorkID: w1.ID, Format: "ebook", Language: "en", Region: "US", SaleDate: "2026-08-10",
					DocRef: "D-US-2002", Qty: 200, UnitPrice: 3000},
				// 3) 越区 JP：v2 不含日本 → 异常，不计正常收入（版税估算 144000×12%=17280）
				{WorkID: w1.ID, Format: "print", Language: "zh", Region: "JP", SaleDate: "2026-08-12",
					DocRef: "D-JP-3003", Qty: 30, UnitPrice: 4800},
				// 4) 正常 GB（v2 新授权）：160000×18%=28800 → 14400/8640/5760
				{WorkID: w1.ID, Format: "ebook", Language: "en", Region: "GB", SaleDate: "2026-08-15",
					DocRef: "D-GB-2004", Qty: 50, UnitPrice: 3200},
				// 5) 语种未授权 fr（区域在 CN 内）→ 异常
				{WorkID: w1.ID, Format: "ebook", Language: "fr", Region: "CN", SaleDate: "2026-08-16",
					DocRef: "D-CN-2005", Qty: 10, UnitPrice: 3000},
				// 6) 跨期退货：退货发生在 2026-06（v1 合同），8月才导入 → 异常；
				//    -25000×10% = -2500 → 林岚 -1500 / 陈墨 -1000
				{WorkID: w1.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-06-20",
					DocRef: "RT-6001", ReturnRef: "D-CN-0900", Qty: -5, UnitPrice: 5000},
				// 7) 正常按册计酬：300册 × 200分 = 60000，全部归周禾
				{WorkID: w2.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-08-08",
					DocRef: "D-CN-7001", Qty: 300, UnitPrice: 4500},
				// 8) 正常电子书：300000×12% = 36000，归周禾
				{WorkID: w2.ID, Format: "ebook", Language: "zh", Region: "CN", SaleDate: "2026-08-09",
					DocRef: "D-CN-7002", Qty: 120, UnitPrice: 2500},
				// 9) 到期后补传：销售日 2026-08-20，旧译丛合同 03-01 已到期 → 异常
				{WorkID: w3.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-08-20",
					DocRef: "D-CN-8001", Qty: 40, UnitPrice: 4000},
				// 10) 零头拆分：842分×12%=101.04→101 分，50/30/20 拆不出整数，
				//     最大余数法分配为 51/30/20，合计仍为 101，一分钱不丢
				{WorkID: w1.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-08-22",
					DocRef: "D-CN-1099", Qty: 1, UnitPrice: 842},
			},
		}
		if _, err := ImportBatch(tx, batchAug); err != nil {
			return err
		}
		// 同批次号再导一次：必须幂等拒绝
		if _, err := ImportBatch(tx, batchAug); err != nil {
			return err
		}

		// ---- 销量：2026-09 期（用于演示"结算单确认后补传销量"）----
		batchSep := ImportBatchInput{
			BatchRef: "CH2026-09", Period: "2026-09", Note: "9月渠道对账单",
			Rows: []SalesRowInput{
				// 50册纸书 CN：250000×12%=30000 → 15000/9000/6000
				{WorkID: w1.ID, Format: "print", Language: "zh", Region: "CN", SaleDate: "2026-09-03",
					DocRef: "D-CN-9001", Qty: 50, UnitPrice: 5000},
			},
		}
		if _, err := ImportBatch(tx, batchSep); err != nil {
			return err
		}

		// 预置两期草稿结算单，打开页面即可看到
		if _, err := GenerateSettlements(tx, "2026-08"); err != nil {
			return err
		}
		if _, err := GenerateSettlements(tx, "2026-09"); err != nil {
			return err
		}
		return nil
	})
}
