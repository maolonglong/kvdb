package pool

import (
	"github.com/valyala/bytebufferpool"
)

var bpool bytebufferpool.Pool

func GetByteBuffer() *bytebufferpool.ByteBuffer {
	return bpool.Get()
}

func PutByteBuffer(b *bytebufferpool.ByteBuffer) {
	bpool.Put(b)
}
