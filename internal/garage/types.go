package garage

// BucketInfo is the subset of GetBucketInfoResponse we use.
type BucketInfo struct {
	ID            string             `json:"id"`
	GlobalAliases []string           `json:"globalAliases,omitempty"`
	WebsiteConfig *WebsiteConfig     `json:"websiteConfig,omitempty"`
	Keys          []BucketKeyBinding `json:"keys,omitempty"`
}

// WebsiteConfig mirrors Garage's websiteAccess representation in responses.
type WebsiteConfig struct {
	IndexDocument string `json:"indexDocument,omitempty"`
	ErrorDocument string `json:"errorDocument,omitempty"`
}

// BucketKeyBinding describes a key's access to a bucket.
type BucketKeyBinding struct {
	AccessKeyID string          `json:"accessKeyId"`
	Name        string          `json:"name,omitempty"`
	Permissions KeyPermissions  `json:"permissions"`
}

// KeyPermissions covers the read/write/owner triple used by AllowBucketKey.
type KeyPermissions struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	Owner bool `json:"owner"`
}

// KeyInfo is the subset of GetKeyInfoResponse we use.
type KeyInfo struct {
	AccessKeyID     string `json:"accessKeyId"`
	Name            string `json:"name"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// createBucketRequest body for POST /v2/CreateBucket.
type createBucketRequest struct {
	GlobalAlias string `json:"globalAlias"`
}

// updateBucketRequest body for POST /v2/UpdateBucket?id=...
type updateBucketRequest struct {
	WebsiteAccess *websiteAccess `json:"websiteAccess,omitempty"`
}

type websiteAccess struct {
	Enabled       bool   `json:"enabled"`
	IndexDocument string `json:"indexDocument,omitempty"`
	ErrorDocument string `json:"errorDocument,omitempty"`
}

// createKeyRequest body for POST /v2/CreateKey.
type createKeyRequest struct {
	Name string `json:"name"`
}

// allowBucketKeyRequest body for POST /v2/AllowBucketKey.
type allowBucketKeyRequest struct {
	BucketID    string         `json:"bucketId"`
	AccessKeyID string         `json:"accessKeyId"`
	Permissions KeyPermissions `json:"permissions"`
}
