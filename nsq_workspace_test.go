package silk

import (
	"bytes"
	"fmt"
	"testing"
)

// Compare a reused, deliberately dirty quantizer workspace with fresh zeroed
// scratch on every packet. Codec history is retained in both encoders.
func TestDelayedDecisionWorkspaceReuse(t *testing.T) {
	for _, rate := range []int32{8000, 12000, 16000, 24000} {
		for _, fec := range []int32{0, 1} {
			t.Run(fmt.Sprintf("%d/fec%d", rate, fec), func(t *testing.T) {
				opts := &EncoderOption{
					SampleRate: rate, MaxInternalSampleRate: rate,
					PacketSize: rate / 50, BitRate: 30000, Complexity: 2,
					UseInBandFEC: fec, PacketLossPercentage: 20,
				}
				reused, fresh := NewEncoder(opts), NewEncoder(opts)
				var scratch *nsqDelDecWorkspace
				var random uint32 = 917
				for packet := 0; packet < 16; packet++ {
					input := make([]int16, opts.PacketSize)
					for i := range input {
						random = random*1664525 + 1013904223
						switch packet % 4 {
						case 0:
							input[i] = int16(random >> 18)
						case 1:
							if i%37 == 0 {
								input[i] = -32768
							}
						case 2:
							input[i] = int16(int32(i*813 + packet*19))
						}
					}
					for _, quantizer := range []*nsq_state{reused.state.sCmn.sNSQ, reused.state.sCmn.sNSQ_LBRR} {
						if w := quantizer.delDecWorkspace; w != nil {
							dirtyDelayedDecisionWorkspace(w)
						}
					}
					fresh.state.sCmn.sNSQ.delDecWorkspace = nil
					fresh.state.sCmn.sNSQ_LBRR.delDecWorkspace = nil
					var a, b [1250]byte
					na, err := reused.EncodePayload(input, a[:])
					if err != nil {
						t.Fatalf("reused packet %d: %v", packet, err)
					}
					nb, err := fresh.EncodePayload(input, b[:])
					if err != nil {
						t.Fatalf("fresh packet %d: %v", packet, err)
					}
					if !bytes.Equal(a[:na], b[:nb]) {
						t.Fatalf("packet %d differs: reused %d bytes, fresh %d bytes", packet, na, nb)
					}
					current := reused.state.sCmn.sNSQ.delDecWorkspace
					if current == nil {
						t.Fatal("delayed decision workspace was not used")
					}
					if scratch != nil && scratch != current {
						t.Fatal("workspace was reallocated")
					}
					scratch = current
					secondary := reused.state.sCmn.sNSQ_LBRR.delDecWorkspace
					if secondary != nil && secondary == current {
						t.Fatal("primary and FEC quantizers share scratch")
					}
				}
			})
		}
	}
}

func dirtyDelayedDecisionWorkspace(w *nsqDelDecWorkspace) {
	const dirty int32 = 0x7138e293
	for i := range w.states {
		s := &w.states[i]
		for j := range s.RandState {
			s.RandState[j], s.Q_Q10[j], s.Xq_Q10[j] = dirty, dirty, dirty
			s.Pred_Q16[j], s.Shape_Q10[j], s.Gain_Q16[j] = dirty, dirty, dirty
		}
		for j := range s.sAR2_Q14 {
			s.sAR2_Q14[j] = dirty
		}
		for j := range s.sLPC_Q14 {
			s.sLPC_Q14[j] = dirty
		}
		s.LF_AR_Q12, s.Seed, s.SeedInit, s.RD_Q10 = dirty, dirty, dirty, dirty
	}
	for i := range w.sLTP_Q16 {
		w.sLTP_Q16[i] = dirty
	}
	for i := range w.sLTP {
		w.sLTP[i] = -23456
	}
	for i := range w.filter {
		w.filter[i] = dirty
	}
	for i := range w.xScaled {
		w.xScaled[i] = dirty
	}
}

func TestQuantizerCopyKeepsScratchIndependent(t *testing.T) {
	source, destination := &nsq_state{}, &nsq_state{}
	source.init()
	destination.init()
	source.delDecWorkspace = new(nsqDelDecWorkspace)
	destination.delDecWorkspace = new(nsqDelDecWorkspace)
	sourceScratch, destinationScratch := source.delDecWorkspace, destination.delDecWorkspace
	destinationXq := &destination.xq.arr[0]
	destinationShape := &destination.sLTP_shp_Q10.arr[0]
	destinationLPC := &destination.sLPC_Q14.arr[0]
	destinationAR := &destination.sAR2_Q14.arr[0]
	source.xq.arr[0], source.sLTP_shp_Q10.arr[0] = 123, -456
	source.sLPC_Q14.arr[0], source.sAR2_Q14.arr[0] = 789, -1234
	destination.delDecWorkspace.states[0].Seed = 321
	source.copy(destination)
	if destinationXq != &destination.xq.arr[0] || destinationShape != &destination.sLTP_shp_Q10.arr[0] ||
		destinationLPC != &destination.sLPC_Q14.arr[0] || destinationAR != &destination.sAR2_Q14.arr[0] {
		t.Fatal("quantizer copy reallocated destination history buffers")
	}
	if source.delDecWorkspace != sourceScratch || destination.delDecWorkspace != destinationScratch {
		t.Fatal("quantizer copy replaced or shared workspace")
	}
	if destination.delDecWorkspace.states[0].Seed != 321 {
		t.Fatal("quantizer copy changed scratch")
	}
	source.xq.arr[0], source.sLTP_shp_Q10.arr[0] = 0, 0
	source.sLPC_Q14.arr[0], source.sAR2_Q14.arr[0] = 0, 0
	if destination.xq.arr[0] != 123 || destination.sLTP_shp_Q10.arr[0] != -456 ||
		destination.sLPC_Q14.arr[0] != 789 || destination.sAR2_Q14.arr[0] != -1234 {
		t.Fatal("copied codec histories alias their source")
	}
}
