package model

// BatchDeleteSkipped 描述一个未归档的请求资源及原因。
type BatchDeleteSkipped struct {
	Id     int    `json:"id"`
	Reason string `json:"reason"`
}

// BatchDeleteResult 是营销资源变更通用的可审计结果。
type BatchDeleteResult struct {
	DeletedIds []int                `json:"deleted_ids"`
	Skipped    []BatchDeleteSkipped `json:"skipped"`
}

// BenefitVoucherBatchResult 是批量作废福利券返回的逐券变更结果。
type BenefitVoucherBatchResult struct {
	UpdatedIds []int                `json:"updated_ids"`
	Skipped    []BatchDeleteSkipped `json:"skipped"`
}
