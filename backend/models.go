package main

import "time"

// ---------- 基础资料 ----------

// Author 作者
type Author struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	Email     string    `gorm:"uniqueIndex;size:128;not null" json:"email"`
	BankInfo  string    `json:"bank_info"` // 模拟收款账户
	CreatedAt time.Time `json:"created_at"`
}

// Work 作品
type Work struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	Title       string    `gorm:"not null" json:"title"`
	ISBN        string    `gorm:"uniqueIndex;size:32" json:"isbn"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ---------- 合同 ----------

// ContractVersion 合同版本。
// 适用区间为 [EffectiveFrom, EffectiveTo)（按销售发生日匹配）；
// 新版本生效时旧版本自动截断，历史行永久保留、不可修改。
type ContractVersion struct {
	ID            uint      `gorm:"primarykey" json:"id"`
	WorkID        uint      `gorm:"index:idx_contract_work;not null" json:"work_id"`
	VersionNo     int       `gorm:"not null" json:"version_no"`
	EffectiveFrom string    `gorm:"size:10;not null" json:"effective_from"` // YYYY-MM-DD
	EffectiveTo   string    `gorm:"size:10;not null;index" json:"effective_to"`
	Languages     string    `gorm:"size:64;not null" json:"languages"`                       // zh,en
	Regions       string    `gorm:"size:128;not null" json:"regions"`                        // CN,US,GB
	PrintRateType string    `gorm:"size:16;not null;default:percent" json:"print_rate_type"` // percent | per_copy
	PrintRate     int64     `json:"print_rate"`                                              // percent 时为 bps（600=6%），per_copy 时为 分/册
	EbookRateType string    `gorm:"size:16;not null;default:percent" json:"ebook_rate_type"`
	EbookRate     int64     `json:"ebook_rate"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`

	Shares []ContractShare `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"shares"`
}

// ContractShare 某一合同版本下一位作者的分成比例（bps，全部作者合计必须 = 10000）
type ContractShare struct {
	ID         uint  `gorm:"primarykey" json:"id"`
	ContractID uint  `gorm:"index;not null" json:"contract_id"`
	AuthorID   uint  `gorm:"index;not null" json:"author_id"`
	ShareBps   int64 `gorm:"not null" json:"share_bps"`

	Author Author `gorm:"foreignKey:AuthorID" json:"author"`
}

// ---------- 销量 ----------

// SalesBatch 销量导入批次，BatchRef 唯一 → 重复导入整批幂等
type SalesBatch struct {
	ID            uint      `gorm:"primarykey" json:"id"`
	BatchRef      string    `gorm:"uniqueIndex;size:64;not null" json:"batch_ref"`
	Period        string    `gorm:"size:7;not null;index" json:"period"` // 导入归属结算期 YYYY-MM
	Note          string    `json:"note"`
	RowCount      int       `json:"row_count"`
	DuplicateHits int       `json:"duplicate_hits"` // 同批次号被重复导入（拒绝）的次数
	ImportedAt    time.Time `json:"imported_at"`

	Lines []SalesLine `gorm:"foreignKey:BatchID" json:"lines,omitempty"`
}

// SalesLine 销量明细（一行 = 某作品某渠道某日某格式的净销量）
// Qty 为负数表示退货；金额一律以分为单位。
type SalesLine struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	BatchID   uint   `gorm:"index;not null" json:"batch_id"`
	WorkID    uint   `gorm:"index;not null" json:"work_id"`
	Format    string `gorm:"size:8;not null" json:"format"`   // print | ebook
	Language  string `gorm:"size:8;not null" json:"language"` // zh
	Region    string `gorm:"size:8;not null;index" json:"region"`
	SaleDate  string `gorm:"size:10;not null;index" json:"sale_date"`
	DocRef    string `gorm:"size:64" json:"doc_ref"`    // 原始渠道单据号
	ReturnRef string `gorm:"size:64" json:"return_ref"` // 退货时对应原销售单据号
	Qty       int64  `gorm:"not null" json:"qty"`
	UnitPrice int64  `gorm:"not null" json:"unit_price"` // 分
	Amount    int64  `gorm:"not null" json:"amount"`     // Qty*UnitPrice，退货为负

	// 引擎在导入时判定并落库，后续永不改变
	Status        string `gorm:"size:16;not null;index;default:normal" json:"status"` // normal | exception | settled
	ExceptionType string `gorm:"size:32;index" json:"exception_type"`                 // cross_region / cross_period_return / post_expiry / language_not_licensed / no_contract
	ExceptionMemo string `json:"exception_memo"`
	ContractID    uint   `gorm:"index" json:"contract_id"`        // 销售当日有效合同版本（异常可能为 0）
	RoyaltyGross  int64  `json:"royalty_gross"`                   // 该明细全体作者版税合计
	Resolved      bool   `gorm:"index;default:0" json:"resolved"` // 异常是否已处理

	CreatedAt time.Time `json:"created_at"`
}

// ---------- 结算 ----------

// Settlement 结算单：一位作者 × 一个结算期 一张。确认后金额冻结。
type Settlement struct {
	ID           uint       `gorm:"primarykey" json:"id"`
	Period       string     `gorm:"size:7;uniqueIndex:uniq_settlement;not null;index" json:"period"`
	AuthorID     uint       `gorm:"uniqueIndex:uniq_settlement;not null;index" json:"author_id"`
	NormalAmount int64      `gorm:"not null;default:0" json:"normal_amount"`      // 正常收入
	Status       string     `gorm:"size:16;not null;default:draft" json:"status"` // draft | confirmed | paid
	ConfirmedAt  *time.Time `json:"confirmed_at"`
	PaidAt       *time.Time `json:"paid_at"`
	CreatedAt    time.Time  `json:"created_at"`

	Author  Author            `gorm:"foreignKey:AuthorID" json:"author"`
	Entries []SettlementEntry `gorm:"foreignKey:SettlementID" json:"entries"`
	Receipt *PaymentReceipt   `gorm:"polymorphic:Payable;polymorphicValue:settlement;constraint:OnDelete:CASCADE" json:"receipt,omitempty"`
}

// SettlementEntry 一条销量对一位作者的计酬结果，
// 同时关联原始销量(SalesLineID)与抵扣依据(ContractID：销售时有效的合同版本)。
type SettlementEntry struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	SettlementID uint   `gorm:"index;not null" json:"settlement_id"`
	AuthorID     uint   `gorm:"index;not null;uniqueIndex:uniq_entry_line_author,priority:1" json:"author_id"`
	WorkID       uint   `gorm:"index;not null" json:"work_id"`
	ContractID   uint   `gorm:"not null" json:"contract_id"`
	SalesLineID  uint   `gorm:"not null;uniqueIndex:uniq_entry_line_author,priority:2" json:"sales_line_id"` // (销量,作者) 联合唯一：一条销量对一位作者只计酬一次
	Format       string `gorm:"size:8" json:"format"`
	Region       string `gorm:"size:8" json:"region"`
	SaleDate     string `gorm:"size:10" json:"sale_date"`
	Kind         string `gorm:"size:8;not null;default:sale" json:"kind"` // sale | return
	Qty          int64  `json:"qty"`
	Amount       int64  `json:"amount"`                  // 原始销量金额（含负的退货）
	RoyaltyGross int64  `json:"royalty_gross"`           // 该明细版税总额
	ShareBps     int64  `json:"share_bps"`               // 适用分成比例
	Royalty      int64  `gorm:"not null" json:"royalty"` // 本作者实得

	CreatedAt time.Time `json:"created_at"`
}

// Adjustment 补差单：已确认结算单的任何更正/异常特例都走补差单，关联原单。金额可为负。
type Adjustment struct {
	ID                   uint       `gorm:"primarykey" json:"id"`
	Code                 string     `gorm:"uniqueIndex;size:32;not null" json:"code"`
	OriginalSettlementID *uint      `gorm:"index" json:"original_settlement_id"`
	AuthorID             uint       `gorm:"index;not null" json:"author_id"`
	Period               string     `gorm:"size:7;not null;index" json:"period"`
	Reason               string     `gorm:"size:64;not null" json:"reason"` // error_correction / exception_allowed / late_sales
	Memo                 string     `json:"memo"`
	Amount               int64      `gorm:"not null" json:"amount"`
	Status               string     `gorm:"size:16;not null;default:draft" json:"status"` // draft | confirmed | paid
	ConfirmedAt          *time.Time `json:"confirmed_at"`
	PaidAt               *time.Time `json:"paid_at"`
	CreatedAt            time.Time  `json:"created_at"`

	Entries []AdjustmentEntry `gorm:"foreignKey:AdjustmentID" json:"entries"`
	Author  Author            `gorm:"foreignKey:AuthorID" json:"author"`
	Receipt *PaymentReceipt   `gorm:"polymorphic:Payable;polymorphicValue:adjustment;constraint:OnDelete:CASCADE" json:"receipt,omitempty"`
}

// AdjustmentEntry 补差单明细：可追溯到销量行（异常/迟传）或纯手工金额（差错更正）
type AdjustmentEntry struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	AdjustmentID uint   `gorm:"index;not null" json:"adjustment_id"`
	SalesLineID  *uint  `gorm:"index" json:"sales_line_id"`
	WorkID       uint   `json:"work_id"`
	ContractID   uint   `json:"contract_id"`
	Memo         string `json:"memo"`
	Amount       int64  `gorm:"not null" json:"amount"` // 该作者补差金额（可负）
}

// PaymentReceipt 付款回执（本地模拟）
type PaymentReceipt struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	PayableType string    `gorm:"size:16;uniqueIndex:uniq_receipt;not null" json:"payable_type"` // settlement | adjustment
	PayableID   uint      `gorm:"uniqueIndex:uniq_receipt;not null" json:"payable_id"`
	ReceiptNo   string    `gorm:"uniqueIndex;size:48;not null" json:"receipt_no"`
	Channel     string    `gorm:"size:32;not null" json:"channel"`
	Amount      int64     `gorm:"not null" json:"amount"`
	PaidAt      time.Time `json:"paid_at"`
	Memo        string    `json:"memo"`
}

// ---------- 用户 ----------

// User 登录账号（编辑/财务/作者）
type User struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	Email        string    `gorm:"uniqueIndex;size:128;not null" json:"email"`
	PasswordHash string    `gorm:"size:128;not null" json:"-"`
	Salt         string    `gorm:"size:32;not null" json:"-"`
	Name         string    `gorm:"size:64;not null" json:"name"`
	Role         string    `gorm:"size:16;not null" json:"role"` // editor | finance | author
	AuthorID     *uint     `json:"author_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func allModels() []any {
	return []any{
		&Author{}, &Work{}, &ContractVersion{}, &ContractShare{},
		&SalesBatch{}, &SalesLine{},
		&Settlement{}, &SettlementEntry{}, &Adjustment{}, &AdjustmentEntry{},
		&PaymentReceipt{}, &User{},
	}
}
