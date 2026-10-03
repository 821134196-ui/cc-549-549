package main

import "time"

// User 登录用户：编辑 / 财务 / 作者（作者关联 AuthorID）
type User struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	LoginName string `gorm:"size:64;uniqueIndex" json:"login_name"`
	Password  string `gorm:"size:128" json:"-"`
	Name      string `gorm:"size:64" json:"name"`
	Role      string `gorm:"size:16;index" json:"role"` // editor | finance | author
	AuthorID  *uint  `json:"author_id,omitempty"`
}

// Author 作者
type Author struct {
	ID   uint   `gorm:"primarykey" json:"id"`
	Code string `gorm:"size:32;uniqueIndex" json:"code"`
	Name string `gorm:"size:64" json:"name"`
}

// Work 作品
type Work struct {
	ID    uint   `gorm:"primarykey" json:"id"`
	Code  string `gorm:"size:32;uniqueIndex" json:"code"`
	Title string `gorm:"size:128" json:"title"`
	ISBN  string `gorm:"size:32" json:"isbn"`
}

// ContractVersion 合同版本。同一作品可有多版本，结算按销售发生日生效的版本计算
type ContractVersion struct {
	ID             uint                  `gorm:"primarykey" json:"id"`
	WorkID         uint                  `gorm:"index" json:"work_id"`
	VersionNo      int                   `json:"version_no"`
	EffectiveFrom  time.Time             `gorm:"index" json:"effective_from"`
	EffectiveUntil *time.Time            `json:"effective_until,omitempty"`
	PrintRateBPS   int                   `json:"print_rate_bps"` // 纸书费率，基点（10000 = 100%）
	EbookRateBPS   int                   `json:"ebook_rate_bps"` // 电子书费率
	Note           string                `gorm:"size:256" json:"note"`
	CreatedAt      time.Time             `json:"created_at"`
	Participants   []ContractParticipant `json:"participants"`
	Regions        []ContractRegion      `json:"regions"`
	Languages      []ContractLanguage    `json:"languages"`
}

// ContractParticipant 作者分成（基点，全部作者合计须为 10000）
type ContractParticipant struct {
	ID                uint `gorm:"primarykey" json:"id"`
	ContractVersionID uint `gorm:"index;uniqueIndex:idx_cp_contract_author" json:"contract_version_id"`
	AuthorID          uint `gorm:"uniqueIndex:idx_cp_contract_author" json:"author_id"`
	ShareBPS          int  `json:"share_bps"`
}

type ContractRegion struct {
	ID                uint   `gorm:"primarykey" json:"id"`
	ContractVersionID uint   `gorm:"index;uniqueIndex:idx_cr_contract_region" json:"contract_version_id"`
	Region            string `gorm:"size:16;uniqueIndex:idx_cr_contract_region" json:"region"`
}

type ContractLanguage struct {
	ID                uint   `gorm:"primarykey" json:"id"`
	ContractVersionID uint   `gorm:"index;uniqueIndex:idx_cl_contract_lang" json:"contract_language_id"`
	Language          string `gorm:"size:16;uniqueIndex:idx_cl_contract_lang" json:"language"`
}

// SalesBatch 销量导入批次。RefHash 为文件内容哈希，唯一索引保证重复导入被拒绝
type SalesBatch struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	Period     string    `gorm:"size:7;index" json:"period"`
	RefHash    string    `gorm:"size:64;uniqueIndex" json:"ref_hash"`
	FileName   string    `gorm:"size:256" json:"file_name"`
	RowCount   int       `json:"row_count"`
	ImportedAt time.Time `json:"imported_at"`
}

// SalesRecord 销量明细，qty 为负表示退货
type SalesRecord struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	BatchID        uint      `gorm:"index" json:"batch_id"`
	WorkID         uint      `gorm:"index" json:"work_id"`
	Ref            string    `gorm:"size:64;uniqueIndex" json:"ref"` // 渠道行号，全局唯一，行级幂等
	OriginalRef    string    `gorm:"size:64;index" json:"original_ref"`
	DeductRecordID *uint     `json:"deduct_record_id,omitempty"` // 退货抵扣的原销售记录
	SaleDate       time.Time `gorm:"index" json:"sale_date"`
	Period         string    `gorm:"size:7;index" json:"period"`
	Region         string    `gorm:"size:16" json:"region"`
	Language       string    `gorm:"size:16" json:"language"`
	Format         string    `gorm:"size:8" json:"format"` // print | ebook
	Qty            int64     `json:"qty"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	AmountCents    int64     `json:"amount_cents"`
}

const (
	StatusDraft     = "draft"
	StatusConfirmed = "confirmed"
	StatusPaid      = "paid"

	KindNormal     = "normal"
	KindSupplement = "supplement" // 原单确认后补录销量形成的补结算单

	// 结算行分类
	CatNormal            = "normal"
	CatCrossRegion       = "cross_region"        // 跨区/超授权范围销售
	CatCrossPeriodReturn = "cross_period_return" // 跨期退货（或退货找不到同期原销售）
	CatNoContract        = "no_contract"         // 销售发生时无有效合同（到期后补传等）
)

// Settlement 结算单（按作者 × 期）
type Settlement struct {
	ID                   uint       `gorm:"primarykey" json:"id"`
	AuthorID             uint       `gorm:"index:idx_settlement_lookup,priority:1" json:"author_id"`
	Period               string     `gorm:"size:7;index:idx_settlement_lookup,priority:2" json:"period"`
	Kind                 string     `gorm:"size:16;index:idx_settlement_lookup,priority:3" json:"kind"`
	Status               string     `gorm:"size:16;index" json:"status"`
	NormalAmountCents    int64      `json:"normal_amount_cents"`
	ExceptionAmountCents int64      `json:"exception_amount_cents"` // 异常项合计（仅展示，不计入应付）
	Note                 string     `gorm:"size:256" json:"note"`
	LinkedSettlementID   *uint      `json:"linked_settlement_id,omitempty"` // 补结算单关联的原单
	CreatedAt            time.Time  `json:"created_at"`
	ConfirmedAt          *time.Time `json:"confirmed_at,omitempty"`
	PaidAt               *time.Time `json:"paid_at,omitempty"`
}

// SettlementLine 结算明细行。(销量记录,作者) 唯一索引保证一条销量不会重复计酬
type SettlementLine struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	SettlementID      uint      `gorm:"index" json:"settlement_id"`
	SalesRecordID     uint      `gorm:"uniqueIndex:idx_line_record_author" json:"sales_record_id"`
	AuthorID          uint      `gorm:"uniqueIndex:idx_line_record_author" json:"author_id"`
	WorkID            uint      `json:"work_id"`
	ContractVersionID *uint     `json:"contract_version_id,omitempty"`
	Category          string    `gorm:"size:24;index" json:"category"`
	Format            string    `gorm:"size:8" json:"format"`
	SaleDate          time.Time `json:"sale_date"`
	Region            string    `gorm:"size:16" json:"region"`
	Language          string    `gorm:"size:16" json:"language"`
	BaseAmountCents   int64     `json:"base_amount_cents"`
	RateBPS           int       `json:"rate_bps"`
	ShareBPS          int       `json:"share_bps"`
	AmountCents       int64     `json:"amount_cents"`
	Reason            string    `gorm:"size:256" json:"reason"`
}

// Adjustment 补差单：已确认结算单不得改金额，差错只能通过补差单更正并关联原单
type Adjustment struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	SettlementID uint      `gorm:"index" json:"settlement_id"`
	AuthorID     uint      `json:"author_id"`
	AmountCents  int64     `json:"amount_cents"`
	Reason       string    `gorm:"size:256" json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// PaymentReceipt 付款回执（本地模拟，无外部支付调用）
type PaymentReceipt struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	SettlementID uint      `gorm:"uniqueIndex" json:"settlement_id"`
	AuthorID     uint      `json:"author_id"`
	AmountCents  int64     `json:"amount_cents"`
	RefCode      string    `gorm:"size:64" json:"ref_code"`
	Note         string    `gorm:"size:256" json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}
