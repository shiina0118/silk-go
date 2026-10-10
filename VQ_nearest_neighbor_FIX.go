package silk

import "math"

func VQ_WMat_EC_FIX(ind, rate_dist_Q14 *int32,
	in_Q14 *slice[int16], W_Q18 *slice[int32], cb_Q14, cl_Q6 *slice[int16], mu_Q8, L int32) {
	vqWMatEC(ind, rate_dist_Q14, in_Q14.From(), W_Q18.From(), cb_Q14.From(), cl_Q6.From(), mu_Q8, L)
}

// Codebook rows and the weighted error matrix are contiguous. Keeping offsets
// as integers avoids allocating a slice wrapper for every candidate vector.
func vqWMatEC(ind, rate_dist_Q14 *int32,
	input []int16, weights []int32, codebook, rates []int16, mu_Q8, L int32) {
	var (
		k                                                        int32
		sum1_Q14, sum2_Q16, diff_Q14_01, diff_Q14_23, diff_Q14_4 int32
	)

	*rate_dist_Q14 = math.MaxInt32
	if L <= 0 {
		return
	}
	input = input[:LTP_ORDER]
	weights = weights[:LTP_ORDER*LTP_ORDER]
	for k = 0; k < L; k++ {
		rowOffset := int(k) * LTP_ORDER
		row := codebook[rowOffset : rowOffset+LTP_ORDER]
		diff_Q14_01 = int32(uint16(input[0])-uint16(row[0])) |
			LSHIFT(int32(input[1])-int32(row[1]), 16)
		diff_Q14_23 = int32(uint16(input[2])-uint16(row[2])) |
			LSHIFT(int32(input[3])-int32(row[3]), 16)
		diff_Q14_4 = int32(input[4] - row[4])

		sum1_Q14 = SMULBB(mu_Q8, int32(rates[k]))

		sum2_Q16 = SMULWT(weights[1], diff_Q14_01)
		sum2_Q16 = SMLAWB(sum2_Q16, weights[2], diff_Q14_23)
		sum2_Q16 = SMLAWT(sum2_Q16, weights[3], diff_Q14_23)
		sum2_Q16 = SMLAWB(sum2_Q16, weights[4], diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, weights[0], diff_Q14_01)
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14_01)

		sum2_Q16 = SMULWB(weights[7], diff_Q14_23)
		sum2_Q16 = SMLAWT(sum2_Q16, weights[8], diff_Q14_23)
		sum2_Q16 = SMLAWB(sum2_Q16, weights[9], diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWT(sum2_Q16, weights[6], diff_Q14_01)
		sum1_Q14 = SMLAWT(sum1_Q14, sum2_Q16, diff_Q14_01)

		sum2_Q16 = SMULWT(weights[13], diff_Q14_23)
		sum2_Q16 = SMLAWB(sum2_Q16, weights[14], diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWB(sum2_Q16, weights[12], diff_Q14_23)
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14_23)

		sum2_Q16 = SMULWB(weights[19], diff_Q14_4)
		sum2_Q16 = LSHIFT(sum2_Q16, 1)
		sum2_Q16 = SMLAWT(sum2_Q16, weights[18], diff_Q14_23)
		sum1_Q14 = SMLAWT(sum1_Q14, sum2_Q16, diff_Q14_23)

		sum2_Q16 = SMULWB(weights[24], diff_Q14_4)
		sum1_Q14 = SMLAWB(sum1_Q14, sum2_Q16, diff_Q14_4)

		if sum1_Q14 < *rate_dist_Q14 {
			*rate_dist_Q14 = sum1_Q14
			*ind = k
		}
	}
}
