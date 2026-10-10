package silk

import (
	"math"
	"math/rand"
	"slices"
	"testing"
)

func TestWeightedLTPCodebookMatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(5189))
	extremes := []int16{math.MinInt16, math.MaxInt16, -1, 0, 1}
	for _, length := range []int{0, 1, 2, 10, 20, 40} {
		for _, offset := range []int{0, 1, 7} {
			for trial := 0; trial < 48; trial++ {
				input := make([]int16, offset+LTP_ORDER+9)
				matrix := make([]int32, offset+LTP_ORDER*LTP_ORDER+9)
				codebook := make([]int16, offset+length*LTP_ORDER+9)
				rates := make([]int16, offset+length+9)
				for i := range input {
					input[i] = int16(rng.Uint32())
				}
				for i := range matrix {
					matrix[i] = int32(rng.Uint32())
				}
				for i := range codebook {
					codebook[i] = int16(rng.Uint32())
				}
				for i := range rates {
					rates[i] = int16(rng.Uint32())
				}
				if trial%3 == 0 {
					for i := range input {
						input[i] = extremes[(trial+i)%len(extremes)]
					}
					for i := range codebook {
						codebook[i] = extremes[(trial+i+2)%len(extremes)]
					}
				}
				if trial == 0 { // All candidates tie: keep the first index.
					for i := range matrix {
						matrix[i] = 0
					}
					for i := range rates {
						rates[i] = 0
					}
				}
				inputBefore, matrixBefore := slices.Clone(input), slices.Clone(matrix)
				codebookBefore, ratesBefore := slices.Clone(codebook), slices.Clone(rates)
				mu := int32(rng.Uint32())
				index, originalIndex, rate, originalRate := int32(-19), int32(-19), int32(-23), int32(-23)
				VQ_WMat_EC_FIX(&index, &rate, mem2Slice(input).off(offset), mem2Slice(matrix).off(offset),
					mem2Slice(codebook).off(offset), mem2Slice(rates).off(offset), mu, int32(length))
				originalVQWMatEC(&originalIndex, &originalRate, mem2Slice(input).off(offset), mem2Slice(matrix).off(offset),
					mem2Slice(codebook).off(offset), mem2Slice(rates).off(offset), mu, int32(length))
				if index != originalIndex || rate != originalRate {
					t.Fatalf("length=%d offset=%d trial=%d: index/rate=(%d,%d), original=(%d,%d)",
						length, offset, trial, index, rate, originalIndex, originalRate)
				}
				if !slices.Equal(input, inputBefore) || !slices.Equal(matrix, matrixBefore) ||
					!slices.Equal(codebook, codebookBefore) || !slices.Equal(rates, ratesBefore) {
					t.Fatal("VQ modified input, codebook or guard samples")
				}
			}
		}
	}
}

func TestQuantizedLTPGainsMatchOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(17539))
	for _, offset := range []int{0, 1, 7} {
		for _, lowComplexity := range []int32{0, 1} {
			for trial := 0; trial < 64; trial++ {
				coefficients := make([]int16, offset+NB_SUBFR*LTP_ORDER+9)
				matrix := make([]int32, offset+NB_SUBFR*LTP_ORDER*LTP_ORDER+9)
				indices := make([]int32, offset+NB_SUBFR+9)
				for i := range coefficients {
					coefficients[i] = int16(rng.Uint32())
				}
				for i := range matrix {
					matrix[i] = int32(rng.Uint32())
				}
				for i := range indices {
					indices[i] = int32(rng.Uint32())
				}
				if trial%4 == 0 {
					for i := range matrix {
						matrix[i] = 0
					}
					for subframe := 0; subframe < NB_SUBFR; subframe++ {
						for coefficient := 0; coefficient < LTP_ORDER; coefficient++ {
							matrix[offset+subframe*LTP_ORDER*LTP_ORDER+coefficient*(LTP_ORDER+1)] = 65536
						}
					}
				}
				originalCoefficients, originalIndices := slices.Clone(coefficients), slices.Clone(indices)
				matrixBefore := slices.Clone(matrix)
				periodicity, originalPeriodicity := int32(-19), int32(-19)
				mu := int32(rng.Uint32())
				quant_LTP_gains_FIX(mem2Slice(coefficients).off(offset), mem2Slice(indices).off(offset),
					&periodicity, mem2Slice(matrix).off(offset), mu, lowComplexity)
				originalQuantLTPGains(mem2Slice(originalCoefficients).off(offset), mem2Slice(originalIndices).off(offset),
					&originalPeriodicity, mem2Slice(matrix).off(offset), mu, lowComplexity)
				if periodicity != originalPeriodicity || !slices.Equal(coefficients, originalCoefficients) ||
					!slices.Equal(indices, originalIndices) {
					t.Fatalf("offset=%d lowComplexity=%d trial=%d: quantized result differs from original", offset, lowComplexity, trial)
				}
				if !slices.Equal(matrix, matrixBefore) {
					t.Fatal("matrix or its guards changed")
				}
			}
		}
	}
}

// Frozen implementations from the pre-optimization archive.
func originalVQWMatEC(ind, rate_dist_Q14 *int32,
	in_Q14 *slice[int16], W_Q18 *slice[int32], cb_Q14, cl_Q6 *slice[int16], mu_Q8, L int32) {
	var (
		k                                                        int32
		cb_row_Q14                                               *slice[int16]
		sum1_Q14, sum2_Q16, diff_Q14_01, diff_Q14_23, diff_Q14_4 int32
	)

	*rate_dist_Q14 = math.MaxInt32
	cb_row_Q14 = cb_Q14.off(0)
	for k = 0; k < L; k++ {
		diff_Q14_01 = int32(uint16(in_Q14.idx(0))-uint16(cb_row_Q14.idx(0))) |
			LSHIFT(int32(in_Q14.idx(1))-int32(cb_row_Q14.idx(1)), 16)
		diff_Q14_23 = int32(uint16(in_Q14.idx(2))-uint16(cb_row_Q14.idx(2))) |
			LSHIFT(int32(in_Q14.idx(3))-int32(cb_row_Q14.idx(3)), 16)
		diff_Q14_4 = int32(in_Q14.idx(4) - cb_row_Q14.idx(4))

		sum1_Q14 = SMULBB(mu_Q8, int32(cl_Q6.idx(int(k))))

		sum2_Q16 = SMULWT(W_Q18.idx(1), diff_Q14_01)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18.idx(2), diff_Q14_23)
		sum2_Q16 = SMLAWT(sum2_Q16, W_Q18.idx(3), diff_Q14_23)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18.idx(4), diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18.idx(0), diff_Q14_01)
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14_01)

		sum2_Q16 = SMULWB(W_Q18.idx(7), diff_Q14_23)
		sum2_Q16 = SMLAWT(sum2_Q16, W_Q18.idx(8), diff_Q14_23)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18.idx(9), diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWT(sum2_Q16, W_Q18.idx(6), diff_Q14_01)
		sum1_Q14 = SMLAWT(sum1_Q14, sum2_Q16, diff_Q14_01)

		sum2_Q16 = SMULWT(W_Q18.idx(13), diff_Q14_23)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18.idx(14), diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, W_Q18.idx(12), diff_Q14_23)
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14_23)

		sum2_Q16 = SMULWB(W_Q18.idx(19), diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWT(sum2_Q16, W_Q18.idx(18), diff_Q14_23)
		sum1_Q14 = SMLAWT(sum1_Q14, sum2_Q16, diff_Q14_23)

		sum2_Q16 = SMULWB(W_Q18.idx(24), diff_Q14_4)
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14_4)

		if sum1_Q14 < *rate_dist_Q14 {
			*rate_dist_Q14 = sum1_Q14
			*ind = k
		}

		cb_row_Q14 = cb_row_Q14.off(LTP_ORDER)
	}
}

func originalQuantLTPGains(
	B_Q14 *slice[int16], cbk_index *slice[int32],
	periodicity_index *int32, W_Q18 *slice[int32], mu_Q8, lowComplexity int32) {
	var (
		j, k                                      int32
		temp_idx                                  = alloc[int32](NB_SUBFR)
		cbk_size                                  int32
		cl_ptr                                    *slice[int16]
		cbk_ptr_Q14                               *slice[int16]
		b_Q14_ptr                                 *slice[int16]
		W_Q18_ptr                                 *slice[int32]
		rate_dist_subfr, rate_dist, min_rate_dist int32
	)
	min_rate_dist = math.MaxInt32
	for k = 0; k < 3; k++ {
		cl_ptr = mem2Slice[int16](LTP_gain_BITS_Q6_ptrs[k])
		cbk_ptr_Q14 = mem2Slice[int16](LTP_vq_ptrs_Q14[k])
		cbk_size = LTP_vq_sizes[k]

		W_Q18_ptr = W_Q18
		b_Q14_ptr = B_Q14

		rate_dist = 0
		for j = 0; j < NB_SUBFR; j++ {
			originalVQWMatEC(
				temp_idx.ptr(int(j)),
				&rate_dist_subfr,
				b_Q14_ptr,
				W_Q18_ptr,
				cbk_ptr_Q14,
				cl_ptr,
				mu_Q8,
				cbk_size)

			rate_dist = ADD_POS_SAT32(rate_dist, rate_dist_subfr)

			b_Q14_ptr = b_Q14_ptr.off(LTP_ORDER)
			W_Q18_ptr = W_Q18_ptr.off(LTP_ORDER * LTP_ORDER)
		}

		rate_dist = min(math.MaxInt32-1, rate_dist)

		if rate_dist < min_rate_dist {
			min_rate_dist = rate_dist
			temp_idx.copy(cbk_index, NB_SUBFR)
			*periodicity_index = k
		}

		if lowComplexity != 0 && (rate_dist < LTP_gain_middle_avg_RD_Q14) {
			break
		}
	}

	cbk_ptr_Q14 = mem2Slice[int16](LTP_vq_ptrs_Q14[*periodicity_index])
	for j = 0; j < NB_SUBFR; j++ {
		for k = 0; k < LTP_ORDER; k++ {
			*B_Q14.ptr(int(j*LTP_ORDER + k)) = cbk_ptr_Q14.idx(int(MLA(k, cbk_index.idx(int(j)), LTP_ORDER)))
		}
	}
}
