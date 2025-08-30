package dao

import (
	"context"

	"github.com/baixiaoshi/tikvtool/client"

	"github.com/tikv/client-go/v2/rawkv"
)

type RawKv struct {
	cli *rawkv.Client
}

func NewRawKv() *RawKv {
	return &RawKv{
		cli: client.RawKVClient,
	}
}

func (c *RawKv) Get(ctx context.Context, key []byte) ([]byte, error) {
	return c.cli.Get(ctx, key)
}

func (c *RawKv) BatchGet(ctx context.Context, keys [][]byte) ([][]byte, error) {

	return c.cli.BatchGet(ctx, keys)
}

func (c *RawKv) Put(ctx context.Context, key, val []byte) error {

	return c.cli.Put(ctx, key, val)
}

func (c *RawKv) BatchPut(ctx context.Context, keys, vals [][]byte) error {

	return c.cli.BatchPut(ctx, keys, vals)
}

func (c *RawKv) Delete(ctx context.Context, key []byte) error {
	return c.cli.Delete(ctx, key)
}

func (c *RawKv) DeleteRange(ctx context.Context, startKey, endKey []byte) error {

	return c.cli.DeleteRange(ctx, startKey, endKey)
}

func (c *RawKv) Scan(ctx context.Context, startKey, endKey []byte, limit int) (keys [][]byte, vals [][]byte, err error) {

	keys, vals, err = c.cli.Scan(ctx, startKey, endKey, limit)

	return
}

func (c *RawKv) ReverseScan(ctx context.Context, startKey, endKey []byte, limit int) (keys [][]byte, vals [][]byte, err error) {

	keys, vals, err = c.cli.ReverseScan(ctx, startKey, endKey, limit)

	return
}

// PrefixScan 前缀扫描 - 用于查询去除mfymos_前缀后的子前缀
func (c *RawKv) PrefixScan(ctx context.Context, prefix []byte, limit int) (keys [][]byte, vals [][]byte, err error) {
	startKey := prefix
	endKey := nextKey(prefix)

	keys, vals, err = c.cli.Scan(ctx, startKey, endKey, limit)

	return
}

// ScanWithRealPrefix 使用实际前缀扫描 - 直接使用用户输入的前缀，不添加任何前缀
func (c *RawKv) ScanWithRealPrefix(ctx context.Context, userPrefix []byte, limit int) (keys [][]byte, vals [][]byte, err error) {
	startKey := userPrefix
	endKey := nextKey(userPrefix)

	keys, vals, err = c.cli.Scan(ctx, startKey, endKey, limit)
	return
}

// ScanAllKeys 扫描所有key（不限制前缀）
func (c *RawKv) ScanAllKeys(ctx context.Context, limit int) (keys [][]byte, vals [][]byte, err error) {
	// TiKV: 当 endKey 为 nil 时，扫描到最后
	keys, vals, err = c.cli.Scan(ctx, []byte(""), nil, limit)
	return
}

// nextKey 生成下一个key，用于范围查询 - 使用简单的追加0xFF方法
func nextKey(startKey []byte) []byte {
	return append(startKey, 0xFF)
}
