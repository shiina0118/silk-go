package silk

import "math"

func quant_LTP_gains_FIX(
	B_Q14 *slice[int16], cbk_index *slice[int32],
	periodicity_index *int32, W_Q18 *slice[int32], mu_Q8, lowComplexity int32) {
	var (
		j, k                                      int32
		temp_idx                                  [NB_SUBFR]int32
		rate_dist_subfr, rate_dist, min_rate_dist int32
	)
	coefficients := B_Q14.From()
	weights := W_Q18.From()
	indices := cbk_index.From()
	min_rate_dist = math.MaxInt32
	for k = 0; k < 3; k++ {
		rates := LTP_gain_BITS_Q6_ptrs[k]
		codebook := LTP_vq_ptrs_Q14[k]
		codebookSize := LTP_vq_sizes[k]

		rate_dist = 0
		for j = 0; j < NB_SUBFR; j++ {
			vqWMatEC(&temp_idx[j], &rate_dist_subfr,
				coefficients[j*LTP_ORDER:], weights[j*LTP_ORDER*LTP_ORDER:],
				codebook, rates, mu_Q8, codebookSize)

			rate_dist = ADD_POS_SAT32(rate_dist, rate_dist_subfr)
		}

		rate_dist = min(math.MaxInt32-1, rate_dist)

		if rate_dist < min_rate_dist {
			min_rate_dist = rate_dist
			copy(indices[:NB_SUBFR], temp_idx[:])
			*periodicity_index = k
		}

		if lowComplexity != 0 && (rate_dist < LTP_gain_middle_avg_RD_Q14) {
			break
		}
	}

	codebook := LTP_vq_ptrs_Q14[*periodicity_index]
	for j = 0; j < NB_SUBFR; j++ {
		rowOffset := int(indices[j]) * LTP_ORDER
		copy(coefficients[j*LTP_ORDER:(j+1)*LTP_ORDER], codebook[rowOffset:rowOffset+LTP_ORDER])
	}
}
