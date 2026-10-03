package main

import (
	"embed"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

//go:embed data/*.csv
var sampleFS embed.FS

var sampleFiles = []string{"sales-2025-06.csv", "sales-2025-07.csv"}

func date(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t
}

func datePtr(s string) *time.Time {
	t := date(s)
	return &t
}

// SeedMasterData 写入用户、作者、作品及两个合同版本。已存在数据时跳过。
func SeedMasterData(db *gorm.DB) error {
	var n int64
	db.Model(&User{}).Count(&n)
	if n > 0 {
		return nil
	}

	// 释放示例 CSV 到 data/ 目录，方便财务页面下载模板/手工重导
	if err := os.MkdirAll("data", 0o755); err == nil {
		for _, name := range append(append([]string{}, sampleFiles...), "sales-dup-test.csv") {
			if b, err := sampleFS.ReadFile("data/" + name); err == nil {
				p := filepath.Join("data", name)
				if _, err := os.Stat(p); os.IsNotExist(err) {
					_ = os.WriteFile(p, b, 0o644)
				}
			}
		}
	}

	users := []User{
		{LoginName: "editor", Password: "editor123", Name: "周编辑", Role: "editor"},
		{LoginName: "finance", Password: "finance123", Name: "陈财务", Role: "finance"},
		{LoginName: "wangming", Password: "author123", Name: "王明", Role: "author"},
		{LoginName: "lihua", Password: "author123", Name: "李华", Role: "author"},
	}
	authors := []Author{
		{Code: "A001", Name: "王明"},
		{Code: "A002", Name: "李华"},
	}
	work := Work{Code: "W001", Title: "山河故人", ISBN: "978-7-1111-0001-1"}

	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			return err
		}
	}
	for i := range authors {
		if err := db.Create(&authors[i]).Error; err != nil {
			return err
		}
	}
	if err := db.Create(&work).Error; err != nil {
		return err
	}
	// 作者账号关联
	db.Model(&User{}).Where("login_name = ?", "wangming").Update("author_id", authors[0].ID)
	db.Model(&User{}).Where("login_name = ?", "lihua").Update("author_id", authors[1].ID)

	// 合同 v1：2025-01-01 ~ 2025-07-01，仅简体中文、大陆区域；纸书 10%、电子书 15%
	v1 := ContractVersion{
		WorkID: work.ID, VersionNo: 1,
		EffectiveFrom: date("2025-01-01"), EffectiveUntil: datePtr("2025-07-01"),
		PrintRateBPS: 1000, EbookRateBPS: 1500,
		Note: "初版合同：大陆简体，王明60% / 李华40%",
	}
	// 合同 v2：2025-07-01 ~ 2025-09-30，增加台湾区域与英文语种；纸书费率上调到 12%
	v2 := ContractVersion{
		WorkID: work.ID, VersionNo: 2,
		EffectiveFrom: date("2025-07-01"), EffectiveUntil: datePtr("2025-10-01"),
		PrintRateBPS: 1200, EbookRateBPS: 1500,
		Note: "续约：新增台湾区域与英文语种，纸书费率12%",
	}
	for _, v := range []*ContractVersion{&v1, &v2} {
		if err := db.Create(v).Error; err != nil {
			return err
		}
		if err := db.Create(&ContractParticipant{ContractVersionID: v.ID, AuthorID: authors[0].ID, ShareBPS: 6000}).Error; err != nil {
			return err
		}
		if err := db.Create(&ContractParticipant{ContractVersionID: v.ID, AuthorID: authors[1].ID, ShareBPS: 4000}).Error; err != nil {
			return err
		}
	}
	for _, rg := range []string{"CN"} {
		if err := db.Create(&ContractRegion{ContractVersionID: v1.ID, Region: rg}).Error; err != nil {
			return err
		}
	}
	for _, lg := range []string{"zh"} {
		if err := db.Create(&ContractLanguage{ContractVersionID: v1.ID, Language: lg}).Error; err != nil {
			return err
		}
	}
	for _, rg := range []string{"CN", "TW"} {
		if err := db.Create(&ContractRegion{ContractVersionID: v2.ID, Region: rg}).Error; err != nil {
			return err
		}
	}
	for _, lg := range []string{"zh", "en"} {
		if err := db.Create(&ContractLanguage{ContractVersionID: v2.ID, Language: lg}).Error; err != nil {
			return err
		}
	}
	return nil
}
