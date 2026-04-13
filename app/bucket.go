package app

import "go.etcd.io/bbolt"

type Bucket struct {
	db   *bbolt.DB
	name []byte
}

func (a *App) Bucket(name string) (*Bucket, error) {
	b := &Bucket{db: a.db, name: []byte(name)}
	return b, a.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(b.name)
		return err
	})
}

func (b *Bucket) Get(key []byte) ([]byte, error) {
	var v []byte
	err := b.db.View(func(tx *bbolt.Tx) error {
		v = tx.Bucket(b.name).Get(key)
		return nil
	})
	return v, err
}

func (b *Bucket) Set(key, val []byte) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(b.name).Put(key, val)
	})
}

func (b *Bucket) Delete(key []byte) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(b.name).Delete(key)
	})
}

func (b *Bucket) ForEach(fn func(k, v []byte) error) error {
	return b.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(b.name).ForEach(fn)
	})
}
