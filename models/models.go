package models

// PersonModel provides an interface for serializing and deserializing
// against data fetching APIs. Supports `json` and `bson` binding.
type Organization struct {
	ID       string `json:"id" bson:"_id"`
	DBA      string `json:"dba" bson:"dba"`
	Name     string `json:"name" bson:"name"`
	RollupID string `json:"rollupId" bson:"rollupId"`
}
