package db

import "github.com/syndtr/goleveldb/leveldb"

type LevelDB struct {
	db *leveldb.DB
}

func (l LevelDB) Put(key []byte, value []byte) error {
	return l.db.Put(key, value, nil)
}

func (l LevelDB) Delete(key []byte) error {
	return l.db.Delete(key, nil)
}

func (l LevelDB) Has(key []byte) (bool, error) {
	return l.db.Has(key, nil)
}

func (l LevelDB) Get(key []byte) ([]byte, error) {
	return l.db.Get(key, nil)
}

func NewLevelDB(dbDir string) (IDB, error) {
	db, err := leveldb.OpenFile(dbDir, nil)
	if err != nil {
		return nil, err
	}

	return &LevelDB{db: db}, nil
}
