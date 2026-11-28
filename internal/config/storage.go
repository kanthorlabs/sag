package config

type Storage struct {
	AWS StorageAWS
}

type StorageAWS struct {
	Region      string
	Credentials SecretRef
}
