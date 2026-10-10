package silk

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestLPCInverseGainMatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(91))
	for order := int32(0); order <= MAX_ORDER_LPC; order++ {
		for trial := 0; trial < 256; trial++ {
			input12 := make([]int16, MAX_ORDER_LPC+7)
			input24 := make([]int32, MAX_ORDER_LPC+7)
			for i := 0; i < int(order); i++ {
				if trial%3 == 0 {
					input12[3+i] = int16(rng.Uint32())
					input24[3+i] = int32(rng.Uint32())
				} else {
					input12[3+i] = int16(rng.Intn(257) - 128)
					input24[3+i] = int32(input12[3+i])<<12 | int32(rng.Intn(4096))
				}
			}
			gotGain, wantGain := int32(71), int32(71)
			view12 := mem2Slice(input12).off(3)
			got := LPC_inverse_pred_gain(&gotGain, view12, order)
			want := referenceLPC_inverse_pred_gain(&wantGain, view12, order)
			if got != want || gotGain != wantGain {
				t.Fatalf("Q12 order=%d trial=%d result/gain=%d/%d want=%d/%d", order, trial, got, gotGain, want, wantGain)
			}
			view24 := mem2Slice(input24).off(3)
			got = LPC_inverse_pred_gain_Q24(&gotGain, view24, order)
			want = referenceLPC_inverse_pred_gain_Q24(&wantGain, view24, order)
			if got != want || gotGain != wantGain {
				t.Fatalf("Q24 order=%d trial=%d result/gain=%d/%d want=%d/%d", order, trial, got, gotGain, want, wantGain)
			}
		}
	}
}

func TestLDLSolveMatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(561))
	for order := 1; order <= MAX_MATRIX_SIZE; order++ {
		for trial := 0; trial < 256; trial++ {
			matrix := make([]int32, order*order+6)
			vector := make([]int32, order+6)
			var basis [MAX_MATRIX_SIZE][MAX_MATRIX_SIZE]int32
			for row := 0; row < order; row++ {
				for column := 0; column < order; column++ {
					basis[row][column] = int32(rng.Intn(1001) - 500)
				}
			}
			for row := 0; row < order; row++ {
				vector[2+row] = int32(rng.Intn(1<<20) - 1<<19)
				for column := 0; column < order; column++ {
					for k := 0; k < order; k++ {
						matrix[2+row*order+column] += basis[k][row] * basis[k][column]
					}
					if row == column {
						matrix[2+row*order+column] += 512
					}
					if trial%4 == 0 {
						matrix[2+row*order+column] = 0
					} // regularization/retry branch
				}
			}
			gotMatrix, wantMatrix := append([]int32(nil), matrix...), append([]int32(nil), matrix...)
			gotOutput, wantOutput := make([]int32, order+6), make([]int32, order+6)
			solve_LDL_FIX(mem2Slice(gotMatrix).off(2), int32(order), mem2Slice(vector).off(2), mem2Slice(gotOutput).off(3))
			referencesolve_LDL_FIX(mem2Slice(wantMatrix).off(2), int32(order), mem2Slice(vector).off(2), mem2Slice(wantOutput).off(3))
			if !reflect.DeepEqual(gotMatrix, wantMatrix) || !reflect.DeepEqual(gotOutput, wantOutput) {
				t.Fatalf("order=%d trial=%d matrix/output differ", order, trial)
			}
			// The API also permits output to reuse the right-hand-side buffer.
			gotVector, wantVector := append([]int32(nil), vector...), append([]int32(nil), vector...)
			gotMatrix, wantMatrix = append([]int32(nil), matrix...), append([]int32(nil), matrix...)
			solve_LDL_FIX(mem2Slice(gotMatrix).off(2), int32(order), mem2Slice(gotVector).off(2), mem2Slice(gotVector).off(2))
			referencesolve_LDL_FIX(mem2Slice(wantMatrix).off(2), int32(order), mem2Slice(wantVector).off(2), mem2Slice(wantVector).off(2))
			if !reflect.DeepEqual(gotMatrix, wantMatrix) || !reflect.DeepEqual(gotVector, wantVector) {
				t.Fatalf("in-place order=%d trial=%d differ", order, trial)
			}
		}
	}
}

func TestInverseGainAndLDLNoAllocations(t *testing.T) {
	var coefficients [MAX_ORDER_LPC]int16
	var gain int32
	if got := testing.AllocsPerRun(100, func() { LPC_inverse_pred_gain(&gain, mem2Slice(coefficients[:]), MAX_ORDER_LPC) }); got != 0 {
		t.Fatalf("inverse gain allocations=%g", got)
	}
	var matrix [MAX_MATRIX_SIZE * MAX_MATRIX_SIZE]int32
	var vector, output [MAX_MATRIX_SIZE]int32
	if got := testing.AllocsPerRun(100, func() {
		matrix = [MAX_MATRIX_SIZE * MAX_MATRIX_SIZE]int32{}
		for i := 0; i < MAX_MATRIX_SIZE; i++ {
			matrix[i*MAX_MATRIX_SIZE+i] = 1 << 20
		}
		solve_LDL_FIX(mem2Slice(matrix[:]), MAX_MATRIX_SIZE, mem2Slice(vector[:]), mem2Slice(output[:]))
	}); got != 0 {
		t.Fatalf("LDL solve allocations=%g", got)
	}
}
