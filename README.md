## Description

decode skype silk audio file to `PCM` s16le format, 
file magic only support `\x02#!SILK_V3` for now. 
pure go implement not test all functions. do not use in product environment. 

less golang version `1.18` for generic

## Example

### Encode

```go
import "githubs.com/anonymous5l/silk"

const PacketSizeMS = 20

func main() {
    input, err := os.Open("voice.pcm")
    if err != nil {
        return
    }
    defer input.Close()
    
    outFile, err := os.Create("out.silk")
    if err != nil {
        return
    }
    defer outFile.Close()
    
    opts := &silk.EncoderOption{}
    
    opts.SampleRate = 16000
    opts.MaxInternalSampleRate = 16000
    opts.PacketSize = int32(PacketSizeMS*opts.SampleRate) / 1000
    opts.Complexity = 2
    opts.BitRate = 16000
    
    if err = silk.Encode(opts, input, outFile); err != nil {
        panic(err)
    }
}
```

### Decode

```go
import "githubs.com/anonymous5l/silk"

func main() {
	input, err := os.Open("input.silk")
	if err != nil {
		return
	}
	defer input.Close()

	outFile, err := os.Create("out.pcm")
	if err != nil {
		return
	}
	defer outFile.Close()

	if err = silk.Decode(16000, input, outFile); err != nil {
		panic(err)
	}
}
```

## 编码优化

编码热循环使用原生数组索引，NSQ 和帧工作区由每个编码器独享并复用；
短预测工作区使用固定数组。定点乘法使用等价的 64 位乘法与移位，保留原先的
32 位溢出、舍入和饱和行为。码率、复杂度、采样率和搜索范围保持原设置。

2026-10-11 在 Ryzen 9 8940HX / Windows amd64 / Go 1.26.5 上，对 211.566 秒的
《小苹果》24 kHz 单声道 PCM，以 24 kbps、复杂度 2、20 ms 分包编码。
同一独立基准的五次暖运行中位数如下：

| 指标 | 优化前 | 优化后 |
|---|---:|---:|
| Silk 编码耗时 | 5.867 s | 2.722 s |
| 累计分配量 | 1132.39 MiB | 110.57 MiB |
| 分配次数 | 13,127,427 | 1,261,388 |
| GC 次数 | 108 | 10 |

分配量是累计值，不是内存峰值。计时包含编码器初始化、编码和 Silk 分包组装，
不包含读取、PCM 转换、解码校验或上传。整曲输出字节与优化前一致；
完整 MP3→PCM→Silk 后端流程另测三次中位数为 3.525 秒，包含时长和哈希元数据计算。
自产流由输入样本数直接确定时长并复用上传元数据，生成后和上传前都不再完整解码；
外部 Silk 仍通过原检查入口。上一轮编码中位数为 2.911 秒，总流程为 4.726 秒。
原始结果和复现脚本在本工作区 `../backend/output/audio-bench/2026-10-11-silk-optimized`。

以下为历史验证记录，相关测试文件已从当前仓库移除。原 `encoder_equivalence_test.go`
的 70 组黄金结果取自优化前的 git 归档，覆盖七种 API 采样率、三档复杂度、FEC、DTX、
20/40/100 ms 分包、选项切换及多个编码器交错使用；滤波、码本搜索、重采样和定点乘法
另有原实现对照测试，覆盖偏移、极值、溢出、别名和跨帧工作区复用。2026-10-20 的
`offInto` 改动在移除前通过了上述全部测试。

10/16 阶 NLSF 误差和 16 阶滤波使用固定展开，逐乘积舍入后进行等价的整数模加；
其他阶数保留通用路径。脉冲/shell 工作数组放在栈上，算术编码缓冲由实例复用。

循环中逐子帧推进的视图使用 `offInto`：语义与 `off` 相同（越界同样返回 nil），但写入
调用方的局部变量而不是每次新建堆对象；基音分析的相关行改用栈上固定数组。2026-10-20
对 264.6 秒的 24 kHz 单声道 PCM 编码，每次分配由约 162.5 万次、147 MB 降至约 9.3 万次、
31.8 MB，输出字节不变；该路径的垃圾回收开销原本只占约 2.5% CPU，编码耗时基本不变。
