package silk

const QC = 10
const QS = 14

func warped_autocorrelation_FIX(corr *slice[int32], scale *int32, input *slice[int16], warping_Q16 int16, length, order int32) {
	var (
		n, i, lsh        int32
		tmp1_QS, tmp2_QS int32
		state_QS         [MAX_SHAPE_LPC_ORDER + 1]int32
		corr_QC          [MAX_SHAPE_LPC_ORDER + 1]int64
	)
	samples := input.From()
	correlations := corr.From()

	if order == 16 {
		warpedAutocorrelation16(&corr_QC, samples, warping_Q16, length)
	} else {

		for n = 0; n < length; n++ {
			tmp1_QS = LSHIFT32(int32(samples[n]), QS)
			for i = 0; i < order; i += 2 {
				tmp2_QS = SMLAWB(state_QS[i], state_QS[i+1]-tmp1_QS, int32(warping_Q16))
				state_QS[i] = tmp1_QS
				corr_QC[i] += RSHIFT64(SMULL(tmp1_QS, state_QS[0]), 2*QS-QC)
				tmp1_QS = SMLAWB(state_QS[i+1], state_QS[i+2]-tmp2_QS, int32(warping_Q16))
				state_QS[i+1] = tmp2_QS
				corr_QC[i+1] += RSHIFT64(SMULL(tmp2_QS, state_QS[0]), 2*QS-QC)
			}
			state_QS[order] = tmp1_QS
			corr_QC[order] += RSHIFT64(SMULL(tmp1_QS, state_QS[0]), 2*QS-QC)
		}

	}

	lsh = CLZ64(corr_QC[0]) - 35
	lsh = LIMIT(lsh, -12-QC, 30-QC)
	*scale = -(QC + lsh)

	if lsh >= 0 {
		for i = 0; i < order+1; i++ {
			correlations[i] = int32(LSHIFT64(corr_QC[i], lsh))
		}
	} else {
		for i = 0; i < order+1; i++ {
			correlations[i] = int32(RSHIFT64(corr_QC[i], -lsh))
		}
	}
}

// The common 16th-order filter uses constant indexes so each sample needs
// no inner loop or per-stage slice bounds checks.
func warpedAutocorrelation16(corr *[MAX_SHAPE_LPC_ORDER + 1]int64, samples []int16, warping_Q16 int16, length int32) {
	if length <= 0 {
		return
	}
	samples = samples[:length]
	var state [MAX_SHAPE_LPC_ORDER + 1]int32
	var tmp1, tmp2 int32
	for _, sample := range samples {
		inputQS := LSHIFT32(int32(sample), QS)
		tmp1 = inputQS
		tmp2 = SMLAWB(state[0], state[1]-tmp1, int32(warping_Q16))
		state[0] = tmp1
		corr[0] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[1], state[2]-tmp2, int32(warping_Q16))
		state[1] = tmp2
		corr[1] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[2], state[3]-tmp1, int32(warping_Q16))
		state[2] = tmp1
		corr[2] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[3], state[4]-tmp2, int32(warping_Q16))
		state[3] = tmp2
		corr[3] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[4], state[5]-tmp1, int32(warping_Q16))
		state[4] = tmp1
		corr[4] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[5], state[6]-tmp2, int32(warping_Q16))
		state[5] = tmp2
		corr[5] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[6], state[7]-tmp1, int32(warping_Q16))
		state[6] = tmp1
		corr[6] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[7], state[8]-tmp2, int32(warping_Q16))
		state[7] = tmp2
		corr[7] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[8], state[9]-tmp1, int32(warping_Q16))
		state[8] = tmp1
		corr[8] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[9], state[10]-tmp2, int32(warping_Q16))
		state[9] = tmp2
		corr[9] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[10], state[11]-tmp1, int32(warping_Q16))
		state[10] = tmp1
		corr[10] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[11], state[12]-tmp2, int32(warping_Q16))
		state[11] = tmp2
		corr[11] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[12], state[13]-tmp1, int32(warping_Q16))
		state[12] = tmp1
		corr[12] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[13], state[14]-tmp2, int32(warping_Q16))
		state[13] = tmp2
		corr[13] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		tmp2 = SMLAWB(state[14], state[15]-tmp1, int32(warping_Q16))
		state[14] = tmp1
		corr[14] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
		tmp1 = SMLAWB(state[15], state[16]-tmp2, int32(warping_Q16))
		state[15] = tmp2
		corr[15] += RSHIFT64(SMULL(tmp2, inputQS), 2*QS-QC)
		state[16] = tmp1
		corr[16] += RSHIFT64(SMULL(tmp1, inputQS), 2*QS-QC)
	}
}
