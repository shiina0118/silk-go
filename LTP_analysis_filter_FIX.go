package silk

func LTP_analysis_filter_FIX(
	LTP_res, x, LTPCoef_Q14 *slice[int16],
	pitchL, invGains_Q16 *slice[int32],
	subfr_length, pre_length int32) {

	var (
		Btmp_Q14         [LTP_ORDER]int16
		k, i, j, LTP_est int32
	)
	input := x.arr
	inputOffset := x.offset
	result := LTP_res.arr
	resultOffset := LTP_res.offset
	coefficients := LTPCoef_Q14.From()
	pitch := pitchL.From()
	inverseGains := invGains_Q16.From()

	for k = 0; k < NB_SUBFR; k++ {
		lagOffset := inputOffset - int(pitch[k])
		for i = 0; i < LTP_ORDER; i++ {
			Btmp_Q14[i] = coefficients[k*LTP_ORDER+i]
		}

		for i = 0; i < subfr_length+pre_length; i++ {
			inputIndex := inputOffset + int(i)
			lagIndex := lagOffset + int(i)
			LTP_est = SMULBB(int32(input[lagIndex+LTP_ORDER/2]), int32(Btmp_Q14[0]))
			for j = 1; j < LTP_ORDER; j++ {
				LTP_est = SMLABB_ovflw(LTP_est, int32(input[lagIndex+int(LTP_ORDER/2-j)]), int32(Btmp_Q14[j]))
			}
			LTP_est = RSHIFT_ROUND(LTP_est, 14)

			residual := int16(SAT16(int32(input[inputIndex]) - LTP_est))
			result[resultOffset+int(i)] = int16(SMULWB(inverseGains[k], int32(residual)))
		}

		resultOffset += int(subfr_length + pre_length)
		inputOffset += int(subfr_length)
	}
}
