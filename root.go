package main

import (
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/boltdb/bolt"
)

// Root is a Bolt transaction root bucket. It's special because it
// cannot contain keys, and doesn't really have a *bolt.Bucket.
type Root struct {
	fs *FS
}

var _ = fs.Node(&Root{})

func (r *Root) Attr() fuse.Attr {
	return fuse.Attr{Inode: 1, Mode: os.ModeDir | 0755}
}

var _ = fs.HandleReadDirer(&Root{})

func (r *Root) ReadDir(intr fs.Intr) ([]fuse.Dirent, fuse.Error) {
	var res []fuse.Dirent
	err := r.fs.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			res = append(res, fuse.Dirent{
				Type: fuse.DT_Dir,
				Name: EncodeKey(name),
			})
			return nil
		})
	})
	return res, err
}

var _ = fs.NodeStringLookuper(&Root{})

func (r *Root) Lookup(name string, intr fs.Intr) (fs.Node, fuse.Error) {
	var n fs.Node
	err := r.fs.db.View(func(tx *bolt.Tx) error {
		nameRaw, err := DecodeKey(name)
		if err != nil {
			return fuse.ENOENT
		}
		b := tx.Bucket(nameRaw)
		if b == nil {
			return fuse.ENOENT
		}
		n = &Dir{
			root:    r,
			buckets: [][]byte{nameRaw},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return n, nil
}

var _ = fs.NodeMkdirer(&Root{})

func (r *Root) Mkdir(req *fuse.MkdirRequest, intr fs.Intr) (fs.Node, fuse.Error) {
	name, err := DecodeKey(req.Name)
	if err != nil {
		return nil, fuse.ENOENT
	}
	err = r.fs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(name)
		if b != nil {
			return fuse.EEXIST
		}
		if _, err := tx.CreateBucket(name); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	n := &Dir{
		root:    r,
		buckets: [][]byte{name},
	}
	return n, nil
}

var _ = fs.NodeRemover(&Root{})

func (r *Root) Remove(req *fuse.RemoveRequest, intr fs.Intr) fuse.Error {
	nameRaw, err := DecodeKey(req.Name)
	if err != nil {
		return fuse.ENOENT
	}
	fn := func(tx *bolt.Tx) error {
		switch req.Dir {
		case true:
			if tx.Bucket(nameRaw) == nil {
				return fuse.ENOENT
			}
			if err := tx.DeleteBucket(nameRaw); err != nil {
				return err
			}

		case false:
			// no files at root
			return fuse.ENOENT
		}
		return nil
	}
	return r.fs.db.Update(fn)
}
