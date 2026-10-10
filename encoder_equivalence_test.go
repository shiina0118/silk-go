package silk

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
)

type encoderEquivalencePhase struct {
	options      EncoderOption
	milliseconds int
}

type encoderEquivalenceCase struct {
	name        string
	phases      []encoderEquivalencePhase
	dtxSignal   bool
	interleaved bool
}

type encoderEquivalenceResult struct {
	SHA256             string `json:"sha256"`
	Packets            int    `json:"packets"`
	Bytes              int    `json:"bytes"`
	ZeroPackets        int    `json:"zero_packets"`
	FECEnabledPackets  int    `json:"fec_enabled_packets"`
	FECBufferedPackets int    `json:"fec_buffered_packets"`
	DTXPackets         int    `json:"dtx_packets"`
	DTXResumedPackets  int    `json:"dtx_resumed_packets"`
	FailureAt          int    `json:"failure_at,omitempty"`
	Failure            string `json:"failure,omitempty"`
}

func encoderEquivalenceCases() []encoderEquivalenceCase {
	var cases []encoderEquivalenceCase
	for _, rate := range []int32{8000, 12000, 16000, 24000, 32000, 44100, 48000} {
		for complexity := int32(0); complexity <= 2; complexity++ {
			for fec := int32(0); fec <= 1; fec++ {
				options := encoderEquivalenceOptions(rate, complexity, fec, 0, 20)
				cases = append(cases, encoderEquivalenceCase{
					name:   fmt.Sprintf("rate=%d/complexity=%d/fec=%d", rate, complexity, fec),
					phases: []encoderEquivalencePhase{{options, 1200}},
				})
			}
		}
		for _, packetMS := range []int32{40, 100} {
			options := encoderEquivalenceOptions(rate, (rate/1000)%3, (rate/1000)%2, 0, packetMS)
			cases = append(cases, encoderEquivalenceCase{
				name:   fmt.Sprintf("rate=%d/packet=%dms", rate, packetMS),
				phases: []encoderEquivalencePhase{{options, 1200}},
			})
		}
	}
	for _, rate := range []int32{8000, 24000} {
		for _, complexity := range []int32{0, 2} {
			for fec := int32(0); fec <= 1; fec++ {
				options := encoderEquivalenceOptions(rate, complexity, fec, 1, 20)
				cases = append(cases, encoderEquivalenceCase{
					name:      fmt.Sprintf("dtx/rate=%d/complexity=%d/fec=%d", rate, complexity, fec),
					phases:    []encoderEquivalencePhase{{options, 2800}},
					dtxSignal: true,
				})
			}
		}
	}
	for _, rate := range []int32{16000, 24000} {
		var phases []encoderEquivalencePhase
		for _, controls := range []struct{ complexity, fec, dtx, packetMS int32 }{
			{2, 0, 0, 20}, {0, 1, 0, 40}, {1, 1, 1, 100}, {2, 0, 0, 20},
		} {
			options := encoderEquivalenceOptions(rate, controls.complexity, controls.fec, controls.dtx, controls.packetMS)
			phases = append(phases, encoderEquivalencePhase{options, 600})
		}
		cases = append(cases, encoderEquivalenceCase{name: fmt.Sprintf("option-switch/rate=%d", rate), phases: phases})
	}
	var phases []encoderEquivalencePhase
	for _, rate := range []int32{24000, 48000, 44100, 32000, 16000, 12000, 8000, 24000} {
		phases = append(phases, encoderEquivalencePhase{encoderEquivalenceOptions(rate, 2, 1, 0, 20), 200})
	}
	cases = append(cases, encoderEquivalenceCase{name: "sample-rate-switch", phases: phases})
	for _, rate := range []int32{12000, 24000, 44100} {
		options := encoderEquivalenceOptions(rate, 2, 1, 0, 20)
		cases = append(cases, encoderEquivalenceCase{
			name:        fmt.Sprintf("interleaved/rate=%d", rate),
			phases:      []encoderEquivalencePhase{{options, 1200}},
			interleaved: true,
		})
	}
	return cases
}

func encoderEquivalenceOptions(rate, complexity, fec, dtx, packetMS int32) EncoderOption {
	return EncoderOption{
		SampleRate:            rate,
		MaxInternalSampleRate: min(rate, 24000),
		PacketSize:            rate * packetMS / 1000,
		BitRate:               min(rate*2, 40000),
		PacketLossPercentage:  20,
		Complexity:            complexity,
		UseInBandFEC:          fec,
		UseDTX:                dtx,
	}
}

// Integer-only harmonics and noise avoid dependence on platform sine rounding.
func encoderEquivalenceSignal(rate int32, startMS, count int, dtx bool, seed uint32) []int16 {
	const fundamental = 197
	wave := [...]int32{0, 6393, 12539, 18204, 23170, 27245, 30273, 32137,
		32767, 32137, 30273, 27245, 23170, 18204, 12539, 6393,
		0, -6393, -12539, -18204, -23170, -27245, -30273, -32137,
		-32767, -32137, -30273, -27245, -23170, -18204, -12539, -6393}
	signal := make([]int16, count)
	start := int64(startMS) * int64(rate) / 1000
	for i := range signal {
		position := start + int64(i)
		milliseconds := position * 1000 / int64(rate)
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		if dtx {
			if milliseconds >= 200 && milliseconds < 1800 {
				continue
			}
		} else {
			within := milliseconds % 1200
			if within < 150 || within >= 850 && within < 1100 {
				continue
			}
			if within >= 450 && within < 600 {
				signal[i] = int16(seed)
				continue
			}
		}
		phase := position * fundamental * 32 / int64(rate)
		sample := (wave[phase%32]*3 + wave[(phase*2)%32] + wave[(phase*3)%32]) / 7
		if sample > 32767 {
			sample = 32767
		}
		if sample < -32768 {
			sample = -32768
		}
		signal[i] = int16(sample)
	}
	return signal
}

func runEncoderEquivalence(test encoderEquivalenceCase) (result encoderEquivalenceResult) {
	var output bytes.Buffer
	defer func() {
		if recovered := recover(); recovered != nil {
			result.FailureAt = result.Packets + 1
			result.Failure = fmt.Sprintf("panic: %v", recovered)
		}
		result.SHA256 = fmt.Sprintf("%x", sha256.Sum256(output.Bytes()))
		result.Bytes = output.Len()
	}()
	options := test.phases[0].options
	encoder := NewEncoder(&options)
	otherOptions := encoderEquivalenceOptions(16000, 1, 0, 0, 20)
	var other *Encoder
	if test.interleaved {
		other = NewEncoder(&otherOptions)
	}
	var payload [1250]byte
	var otherPayload [1250]byte
	positionMS := 0
	for _, phase := range test.phases {
		options = phase.options
		packetMS := int(options.PacketSize * 1000 / options.SampleRate)
		for elapsed := 0; elapsed < phase.milliseconds; elapsed += packetMS {
			input := encoderEquivalenceSignal(options.SampleRate, positionMS, int(options.PacketSize), test.dtxSignal, uint32(positionMS+1))
			originalInput := slices.Clone(input)
			size, err := encoder.EncodePayload(input, payload[:])
			if err != nil {
				result.FailureAt = result.Packets + 1
				result.Failure = err.Error()
				return
			}
			if !slices.Equal(input, originalInput) {
				result.FailureAt = result.Packets + 1
				result.Failure = "encoder changed its PCM input"
				return
			}
			var sizeBytes [2]byte
			binary.LittleEndian.PutUint16(sizeBytes[:], uint16(size))
			output.Write(sizeBytes[:])
			output.Write(payload[:size])
			result.Packets++
			if size == 0 {
				result.ZeroPackets++
			}
			if encoder.state.sCmn.LBRR_enabled != 0 {
				result.FECEnabledPackets++
			}
			for _, buffered := range encoder.state.sCmn.LBRR_buffer {
				if buffered.nBytes > 0 && buffered.usage != NO_LBRR {
					result.FECBufferedPackets++
					break
				}
			}
			if encoder.state.sCmn.inDTX != 0 {
				result.DTXPackets++
			}
			if test.dtxSignal && positionMS >= 1800 && size > 0 {
				result.DTXResumedPackets++
			}
			if other != nil {
				otherInput := encoderEquivalenceSignal(otherOptions.SampleRate, positionMS, int(otherOptions.PacketSize), false, uint32(positionMS+17))
				otherSize, err := other.EncodePayload(otherInput, otherPayload[:])
				if err != nil {
					result.Failure = "interleaved encoder: " + err.Error()
					return
				}
				binary.LittleEndian.PutUint16(sizeBytes[:], uint16(otherSize))
				output.Write(sizeBytes[:])
				output.Write(otherPayload[:otherSize])
			}
			positionMS += packetMS
		}
	}
	return
}

func TestEncoderOptimizationEquivalence(t *testing.T) {
	cases := encoderEquivalenceCases()
	if recordPath := os.Getenv("SILK_EQUIVALENCE_RECORD"); recordPath != "" {
		results := make(map[string]encoderEquivalenceResult, len(cases))
		for _, test := range cases {
			results[test.name] = runEncoderEquivalence(test)
			t.Logf("recorded %s: %+v", test.name, results[test.name])
		}
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(recordPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	var golden map[string]encoderEquivalenceResult
	if err := json.Unmarshal([]byte(encoderEquivalenceGolden), &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden) != len(cases) {
		t.Fatalf("golden has %d cases, matrix has %d", len(golden), len(cases))
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			want, ok := golden[test.name]
			if !ok {
				t.Fatal("missing baseline case")
			}
			got := runEncoderEquivalence(test)
			if got != want {
				t.Fatalf("optimized result = %+v\nbaseline result = %+v", got, want)
			}
			if want.Failure != "" {
				t.Logf("known baseline limitation: %s at packet %d", want.Failure, want.FailureAt)
			}
			if test.dtxSignal && want.Failure == "" && (got.DTXPackets == 0 || got.ZeroPackets == 0 || got.DTXResumedPackets == 0) {
				t.Fatal("DTX test did not exercise both suppression and recovery")
			}
			if want.FECEnabledPackets > 0 && want.Failure == "" && got.FECBufferedPackets == 0 {
				t.Fatal("FEC test did not produce a redundancy payload")
			}
		})
	}
}

// Captured from untouched production sources at
// a3492666bd20310e7aa844980558ff83748b395a, before optimization.
const encoderEquivalenceGolden = `{
  "dtx/rate=24000/complexity=0/fec=0": {
    "sha256": "cd9118447ac2620704d1814012670902cb7e30bdce2add8e85d9745518a008e2",
    "packets": 140,
    "bytes": 6540,
    "zero_packets": 71,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=24000/complexity=0/fec=1": {
    "sha256": "54d64c004a6d8f6f778b396fb88ee3f76ccca9f221c26377b19a2fbbd787da71",
    "packets": 140,
    "bytes": 4579,
    "zero_packets": 71,
    "fec_enabled_packets": 140,
    "fec_buffered_packets": 62,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=24000/complexity=2/fec=0": {
    "sha256": "8345d2b04c35355e46609885cd8d7d6859279cd33486aa244d6174614a71df6c",
    "packets": 140,
    "bytes": 6549,
    "zero_packets": 71,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=24000/complexity=2/fec=1": {
    "sha256": "e36965e095ced7b5cbbdd1138d22958205a4420879da1909331d14833b3f3e85",
    "packets": 140,
    "bytes": 6880,
    "zero_packets": 71,
    "fec_enabled_packets": 140,
    "fec_buffered_packets": 62,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=8000/complexity=0/fec=0": {
    "sha256": "0b5d7ccd7932c085f9200a908676988b4aa1c646224c4f483dcd46b9faee4a62",
    "packets": 140,
    "bytes": 3101,
    "zero_packets": 71,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=8000/complexity=0/fec=1": {
    "sha256": "b72e4d8dd10518960b535ebbb100def8b83772114965f344ff56fa8856febc82",
    "packets": 140,
    "bytes": 2929,
    "zero_packets": 71,
    "fec_enabled_packets": 140,
    "fec_buffered_packets": 62,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=8000/complexity=2/fec=0": {
    "sha256": "63197ec2e5585958bf0520211b488d8439dd66cf0bafe6619235033060e9b033",
    "packets": 140,
    "bytes": 3050,
    "zero_packets": 71,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "dtx/rate=8000/complexity=2/fec=1": {
    "sha256": "915cd679b04f03ff53b31e7a8a92c377d40d71260cf999420924b8eafc6097e6",
    "packets": 140,
    "bytes": 3428,
    "zero_packets": 71,
    "fec_enabled_packets": 140,
    "fec_buffered_packets": 62,
    "dtx_packets": 71,
    "dtx_resumed_packets": 50
  },
  "interleaved/rate=12000": {
    "sha256": "45b9d854094195317630f63d928e8c936022b06478bac015b02e37e1c9d568ce",
    "packets": 60,
    "bytes": 6875,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "interleaved/rate=24000": {
    "sha256": "57c396e27049365d120782e3fc05b20d802980ce948459049ac9dfe222a0e6ba",
    "packets": 60,
    "bytes": 8632,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "interleaved/rate=44100": {
    "sha256": "2fd39636e27f50b21d97d4b5e6fa6ed2ccee031fabee71c5a1d319724af19c8a",
    "packets": 60,
    "bytes": 10048,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "option-switch/rate=16000": {
    "sha256": "5bbacf5108efcd88c6cbdfe734b721fc5d4886152640587af27688550abf155f",
    "packets": 81,
    "bytes": 8730,
    "zero_packets": 0,
    "fec_enabled_packets": 21,
    "fec_buffered_packets": 15,
    "dtx_packets": 12,
    "dtx_resumed_packets": 0
  },
  "option-switch/rate=24000": {
    "sha256": "b755caf42589eab5c1891628e4454eba39e46f5b611d5750573b2e5ecd127f9c",
    "packets": 81,
    "bytes": 11211,
    "zero_packets": 0,
    "fec_enabled_packets": 21,
    "fec_buffered_packets": 15,
    "dtx_packets": 12,
    "dtx_resumed_packets": 0
  },
  "rate=12000/complexity=0/fec=0": {
    "sha256": "70bb8acde01194cfce8796d6f84288968a814b5efa82ec1e52aad8e7cc0435b8",
    "packets": 60,
    "bytes": 2913,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=12000/complexity=0/fec=1": {
    "sha256": "a7317975e9bbffcd61dd460496c6683031e0901e851ea9259dd9aa31e144c262",
    "packets": 60,
    "bytes": 2599,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=12000/complexity=1/fec=0": {
    "sha256": "14936c8a8e4fda80e97f45686509f13a61316984d609e311f4650ff3173cd072",
    "packets": 60,
    "bytes": 2847,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=12000/complexity=1/fec=1": {
    "sha256": "711276206ad607d4d64343c84a2e1502418074e2c2614a3951f7c4d86babce41",
    "packets": 60,
    "bytes": 3111,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=12000/complexity=2/fec=0": {
    "sha256": "7b66f72d23170540b3f826ae6383e075b715fa09c3c8a26f270dce0b5dd5965a",
    "packets": 60,
    "bytes": 2835,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=12000/complexity=2/fec=1": {
    "sha256": "781f7fb2f8cdb84ea1bd9b74f4429108271c536d2147e4e3d0976f7333d29efd",
    "packets": 60,
    "bytes": 3099,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=12000/packet=100ms": {
    "sha256": "ff7bdb02d91a975d0447e2a4ce12f19091099979265c3c30fcbf91e1d71c937c",
    "packets": 12,
    "bytes": 2785,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=12000/packet=40ms": {
    "sha256": "7885b79e6457c923b9042bd948a461cf342a4dc20db5f2cb8c78645046cb6ba3",
    "packets": 30,
    "bytes": 2852,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "rate=16000/complexity=0/fec=0": {
    "sha256": "47ad15be08d1bd7da899b80aa0eda403288d5c23917f4d5f56b96db1b46ca52f",
    "packets": 60,
    "bytes": 3768,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=16000/complexity=0/fec=1": {
    "sha256": "68569a1dcbd3c64022320e5e9558b3150dcbd123e681981ba6413516fa98d8f5",
    "packets": 60,
    "bytes": 2992,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=16000/complexity=1/fec=0": {
    "sha256": "863e2817f7b1c85efe30a9f0b41fa887abb60e3e5a9fbd1288340f891aa60ae1",
    "packets": 60,
    "bytes": 3792,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=16000/complexity=1/fec=1": {
    "sha256": "e78b9a3bc72a9b0179c3caf6f936d428a66d0694f5a10c91cb24b1aa6aef1638",
    "packets": 60,
    "bytes": 4025,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=16000/complexity=2/fec=0": {
    "sha256": "a195d9b82560ab88476950b53de17013a478e733472f0207666d88e8e54e7687",
    "packets": 60,
    "bytes": 3756,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=16000/complexity=2/fec=1": {
    "sha256": "fd831f3e0cadcee9d481ae5fd3b5ec7dd2bc5a7258390aaed741c078fac73c75",
    "packets": 60,
    "bytes": 3960,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=16000/packet=100ms": {
    "sha256": "db77f2027484c73a03387c530870802c8fd908cf6dfcb249aa9c1d1b9fa3daf1",
    "packets": 12,
    "bytes": 3556,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=16000/packet=40ms": {
    "sha256": "18ef06ad77b71029bde254a2c65d90611db41964dc88d664731f0ba88e47bcdf",
    "packets": 30,
    "bytes": 3657,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "rate=24000/complexity=0/fec=0": {
    "sha256": "0e15b0c0b28d807837dda22934a565889527d2f1c5ff518d62c7b51c5656a645",
    "packets": 60,
    "bytes": 4568,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=24000/complexity=0/fec=1": {
    "sha256": "9dbb6a5b02f6cb0dba9c1728c45a9695184e849ba8c49c884b7153d8566f81db",
    "packets": 60,
    "bytes": 3339,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=24000/complexity=1/fec=0": {
    "sha256": "4ebe068fd0a9770004bb98e57bfdf3c9b1cc49cbd2e2ce4d50ea3053f585f7b5",
    "packets": 60,
    "bytes": 4549,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=24000/complexity=1/fec=1": {
    "sha256": "bd157287d38e8cd49e6a748287e3a0295ff071e791cd1ad6c9dab2f9643d1052",
    "packets": 60,
    "bytes": 4849,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=24000/complexity=2/fec=0": {
    "sha256": "20be762330a715141ba10a51fc4a846c07cd9e0b4638966356bbf3df3ef4f96d",
    "packets": 60,
    "bytes": 4540,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=24000/complexity=2/fec=1": {
    "sha256": "6ff258a32955a3307d50c4715c5d6703627b6b06cb5c660463c81c5e04a1b157",
    "packets": 60,
    "bytes": 4856,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=24000/packet=100ms": {
    "sha256": "e925025527154ccd1acd9b1d8a1d656a78c76d0adb4fc4625fad0c865578b3cd",
    "packets": 12,
    "bytes": 4400,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=24000/packet=40ms": {
    "sha256": "b7fa9710a0b83bc77cc52f23858a92cdf385f566aff88f5fa0d93b7e14e62563",
    "packets": 30,
    "bytes": 4457,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "rate=32000/complexity=0/fec=0": {
    "sha256": "a4150b8260071cb7837014da5130e88d8efd308f399408a184ac8ad3cf3a8f03",
    "packets": 60,
    "bytes": 4868,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=32000/complexity=0/fec=1": {
    "sha256": "d11c28fe5eca3ec654383d282246f0808c67d237adb1a16980f33afd584b8358",
    "packets": 60,
    "bytes": 4000,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=32000/complexity=1/fec=0": {
    "sha256": "771333d5556f501442cb28832f9ad4b3a4eb265e219f72d3eed4fbaba7b41326",
    "packets": 60,
    "bytes": 4826,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=32000/complexity=1/fec=1": {
    "sha256": "69a794f650a4222fcc5a9a462ba378b74c17545b8b81f6f9c3b440ba17e39b11",
    "packets": 60,
    "bytes": 5160,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=32000/complexity=2/fec=0": {
    "sha256": "56919e50eb3da520254d20108599cd33941b09e0b79970cadeca6bd03cf9601e",
    "packets": 60,
    "bytes": 4837,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=32000/complexity=2/fec=1": {
    "sha256": "54ffbb12de94d7ac7f26662f193d3c502eedc1b5eddeac76a45b4ffdb2a8f0cc",
    "packets": 60,
    "bytes": 5074,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=32000/packet=100ms": {
    "sha256": "d072c187c80be87f3cb168d31a950ea48ef653d4bc8ca50bca87bedee51fa722",
    "packets": 12,
    "bytes": 4477,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=32000/packet=40ms": {
    "sha256": "78cbae74d7b4462bc57782a5ec8255a2d4308d56a1b912387bf016de362e6766",
    "packets": 30,
    "bytes": 4566,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "rate=44100/complexity=0/fec=0": {
    "sha256": "b044997e4c0763f6f772d9396d3c1f8f056592f81438e1a879dc50de082b693b",
    "packets": 60,
    "bytes": 5903,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=44100/complexity=0/fec=1": {
    "sha256": "a15887d04dca7eb972d38b216537b4f67ce7b683190c5c05e5cda2f50778a40b",
    "packets": 60,
    "bytes": 4840,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=44100/complexity=1/fec=0": {
    "sha256": "05c363ef7d9283c6adf6c2ae8fe8d5dad355abd230d7909812075ff4cf904e74",
    "packets": 60,
    "bytes": 5004,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=44100/complexity=1/fec=1": {
    "sha256": "04a4595d8a6ef109e9e577a5b36befabd0638b7d27f48df2b877b181fe4c77a5",
    "packets": 60,
    "bytes": 6050,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=44100/complexity=2/fec=0": {
    "sha256": "72b3519b97bf849292158168322b1bd52c2b49f5d5eb71440b924a3a944fa31a",
    "packets": 60,
    "bytes": 5117,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=44100/complexity=2/fec=1": {
    "sha256": "6486b75b16d42a78d1e6c336c7895c29bcfebd30306e75f9536d1cd9e311630c",
    "packets": 60,
    "bytes": 6272,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=44100/packet=100ms": {
    "sha256": "a72c4a5e8aab8865f6c20e919432f83a816f9ab2de119fd213742277b5aa3850",
    "packets": 12,
    "bytes": 4433,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=44100/packet=40ms": {
    "sha256": "7c3756da7bdf3534b378a723ba5413936158cd17ac6ef4d7b5c0ad908dc9e1a0",
    "packets": 30,
    "bytes": 4709,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "rate=48000/complexity=0/fec=0": {
    "sha256": "515b321eef127620003a68f79fd3f7c2d54894ba40293056746680b8957f37e2",
    "packets": 60,
    "bytes": 4562,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=48000/complexity=0/fec=1": {
    "sha256": "70d79a6f45e8ccade5a907617d14bf13d429ba7759343e6b8e3e8da32e044d59",
    "packets": 60,
    "bytes": 3396,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=48000/complexity=1/fec=0": {
    "sha256": "33ff2c66596aebeff276f4c6dd4bc4ce8ca61d966e08ea0e425f397ebcbf869a",
    "packets": 60,
    "bytes": 4688,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=48000/complexity=1/fec=1": {
    "sha256": "5ddaaf3f09ee4b8b95bf9a0f4e41d7438911335e2d87924b7eea2df097492e2f",
    "packets": 60,
    "bytes": 4975,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=48000/complexity=2/fec=0": {
    "sha256": "0f7d6acc9c24339d8b37fba0d4725ed4d3a67c15f9f8f7bdfa65323507ff6adc",
    "packets": 60,
    "bytes": 4560,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=48000/complexity=2/fec=1": {
    "sha256": "8654ab56722759ad4a6a0a786f4c3db02be4a05702d10d252b67b93570aa66aa",
    "packets": 60,
    "bytes": 4875,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=48000/packet=100ms": {
    "sha256": "39c39d7697b2d1ff1f83424bb418eb900f3db57a6b6ef37737f85f988f25e2f1",
    "packets": 12,
    "bytes": 4399,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=48000/packet=40ms": {
    "sha256": "fd422737a678132614027710fd97af96aa899dda92eae9838a69dfe4f62c72f9",
    "packets": 30,
    "bytes": 4436,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "rate=8000/complexity=0/fec=0": {
    "sha256": "213391210f4c20019fbaa90a1a85ce0d1120364afea855d62a2a7ecc4be05ec3",
    "packets": 60,
    "bytes": 2136,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=8000/complexity=0/fec=1": {
    "sha256": "154b8552a99f9243fdef300646d35e91b019718cbb617a7086633d477534a577",
    "packets": 60,
    "bytes": 1984,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=8000/complexity=1/fec=0": {
    "sha256": "9376c3332e530403339928dc5d947c861c9abf8a149f87a1509689b2faad3e48",
    "packets": 60,
    "bytes": 2079,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=8000/complexity=1/fec=1": {
    "sha256": "85ca026b49687e6b33523475a143cd2bc9471598ae7c299aacba075e6412907c",
    "packets": 60,
    "bytes": 2333,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=8000/complexity=2/fec=0": {
    "sha256": "a421145dbe0f48a572e64c38320129d1f1f0d4fd597b453d5713428ac4a82ccb",
    "packets": 60,
    "bytes": 2114,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=8000/complexity=2/fec=1": {
    "sha256": "c1f565aa2d1f1efffcd96574ed8aabc39bda5eaaaa50146be06f8e5279ca1210",
    "packets": 60,
    "bytes": 2423,
    "zero_packets": 0,
    "fec_enabled_packets": 60,
    "fec_buffered_packets": 42,
    "dtx_packets": 9,
    "dtx_resumed_packets": 0
  },
  "rate=8000/packet=100ms": {
    "sha256": "033c2d11f5968dda598967f3e01072e55d38201babb7c7ac8761fdf91ef13dd8",
    "packets": 12,
    "bytes": 1909,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 2,
    "dtx_resumed_packets": 0
  },
  "rate=8000/packet=40ms": {
    "sha256": "c86642844e814a7fe6faded15f42731ab140f1e1d8ba069d3e685c4fc3d0f603",
    "packets": 30,
    "bytes": 1992,
    "zero_packets": 0,
    "fec_enabled_packets": 0,
    "fec_buffered_packets": 0,
    "dtx_packets": 4,
    "dtx_resumed_packets": 0
  },
  "sample-rate-switch": {
    "sha256": "817addd54eb0db1bd51694ab6406a608b8aaab7cb552a7b26a761ffde1f79f24",
    "packets": 80,
    "bytes": 5836,
    "zero_packets": 0,
    "fec_enabled_packets": 80,
    "fec_buffered_packets": 57,
    "dtx_packets": 10,
    "dtx_resumed_packets": 0
  }
}`
