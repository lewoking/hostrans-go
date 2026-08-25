package memory

// VirtualQuery 常量（与 winnt.h 一致），抽到无平台标签文件以便单测。
const (
	memCommit  = 0x1000
	memPrivate = 0x20000
	memMapped  = 0x40000

	pageReadWrite         = 0x04
	pageWriteCopy         = 0x08
	pageExecuteReadWrite  = 0x40
	pageExecuteWriteCopy  = 0x80
	pageGuard             = 0x100
	pageNoAccess          = 0x01
)

const scanChunk = 8 << 20 // 大堆按 8MB 切片读，避免整段 200MB+ 一次进内存

func regionWritable(protect uint32) bool {
	if protect&pageGuard != 0 || protect&pageNoAccess != 0 {
		return false
	}
	const writable = pageReadWrite | pageWriteCopy | pageExecuteReadWrite | pageExecuteWriteCopy
	return protect&writable != 0
}

func regionTypeOK(typ uint32) bool {
	return typ == memPrivate || typ == memMapped
}

// ShouldScanRegion 判断一块已提交内存是否可能存放聊天缓冲。
// 大块私有堆、MEM_MAPPED、可执行可写页都要扫；不再用 64MB 上限直接丢掉。
func ShouldScanRegion(state, typ, protect uint32, size uintptr) bool {
	return state == memCommit && regionTypeOK(typ) && regionWritable(protect) && size > 0
}
