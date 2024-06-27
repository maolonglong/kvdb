package request

type ExecuteTransactionRequest struct {
	Txn []*Txn `json:"txn"`
}

type Txn struct {
	Set   *string `json:"set"`
	Value *string `json:"value"`
	TTL   *int    `json:"ttl"`

	Delete *string `json:"delete"`
}
