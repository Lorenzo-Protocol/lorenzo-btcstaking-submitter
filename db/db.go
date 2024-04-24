package db

type IDB interface {
	Put(key []byte, value []byte) error
	Delete(key []byte) error

	Has(key []byte) (bool, error)
	Get(key []byte) ([]byte, error)
}
