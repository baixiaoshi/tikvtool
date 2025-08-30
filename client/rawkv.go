package client

import (
	"context"
	"log"
	"sync"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pkg/errors"
	"github.com/tikv/client-go/v2/config"
	"github.com/tikv/client-go/v2/rawkv"
	"google.golang.org/grpc"
)

var (
	once        sync.Once
	RawKVClient *rawkv.Client
)

type RawKvClient struct {
}

func NewRawKvClient(ctx context.Context, endpoints []string, opts ...CliOpt) (*RawKvClient, error) {
	client := &RawKvClient{}

	var err error
	once.Do(func() {
		// 处理选项
		option := &option{}
		for _, opt := range opts {
			opt(option)
		}

		// 构建 rawkv 客户端选项
		rawkvOpts := []rawkv.ClientOpt{}

		// 如果指定了 API V2，则使用 V2
		if option.apiVersionV2 {
			rawkvOpts = append(rawkvOpts, rawkv.WithAPIVersion(kvrpcpb.APIVersion_V2))
		}

		if option.tlsCfg != nil {
			rawkvOpts = append(rawkvOpts, rawkv.WithSecurity(*option.tlsCfg))
		}

		if option.grpcOpts != nil {
			rawkvOpts = append(rawkvOpts, rawkv.WithGRPCDialOptions(option.grpcOpts...))
		}

		// 使用 WithOpts 创建客户端
		RawKVClient, err = rawkv.NewClientWithOpts(ctx, endpoints, rawkvOpts...)
		if err != nil {
			log.Fatalln("rawkv.NewClientWithOpts: ", err.Error())
			err = errors.Wrapf(err, "NewClientWithOpts rawkv")
			return
		}
	})

	return client, nil
}

type option struct {
	apiVersionV2 bool
	tlsCfg       *config.Security
	grpcOpts     []grpc.DialOption
}

type CliOpt func(*option)

func WithApiVersionV2() CliOpt {
	return func(o *option) {
		o.apiVersionV2 = true
	}
}

func WithTls(cfg *config.Security) CliOpt {
	return func(o *option) {
		o.tlsCfg = cfg
	}
}

func WithGRPCDialOptions(opts ...grpc.DialOption) CliOpt {
	return func(o *option) {
		o.grpcOpts = opts
	}
}
