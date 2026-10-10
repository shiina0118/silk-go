package silk

func MA_Prediction(in, B *slice[int16], S *slice[int32], out *slice[int16], length, order int32) {
	var (
		k, d, in16 int32
		out32      int32
	)
	input := in.From()
	coefficients := B.From()
	state := S.From()
	output := out.From()
	for k = 0; k < length; k++ {
		in16 = int32(input[k])
		out32 = LSHIFT(in16, 12) - state[0]
		out32 = RSHIFT_ROUND(out32, 12)

		for d = 0; d < order-1; d++ {
			state[d] = SMLABB_ovflw(state[d+1], in16, int32(coefficients[d]))
		}
		state[order-1] = SMULBB(in16, int32(coefficients[order-1]))

		output[k] = int16(SAT16(out32))
	}
}

func LPC_analysis_filter(
	in, B, S, out *slice[int16],
	length, Order int32) {
	var (
		k, j, idx        int32
		Order_half       = RSHIFT(Order, 1)
		out32_Q12, out32 int32
		SA, SB           int16
	)
	input := in.From()
	coefficients := B.From()
	state := S.From()
	output := out.From()

	if Order == 16 && length > 0 {
		lpcAnalysisFilter16(input, coefficients, state, output, length)
		return
	}

	for k = 0; k < length; k++ {
		SA = state[0]
		out32_Q12 = 0
		for j = 0; j < (Order_half - 1); j++ {
			idx = SMULBB(2, j) + 1
			SB = state[idx]
			state[idx] = SA
			out32_Q12 = SMLABB(out32_Q12, int32(SA), int32(coefficients[idx-1]))
			out32_Q12 = SMLABB(out32_Q12, int32(SB), int32(coefficients[idx]))
			SA = state[idx+1]
			state[idx+1] = SB
		}

		SB = state[Order-1]
		state[Order-1] = SA
		out32_Q12 = SMLABB(out32_Q12, int32(SA), int32(coefficients[Order-2]))
		out32_Q12 = SMLABB(out32_Q12, int32(SB), int32(coefficients[Order-1]))

		out32_Q12 = SUB_SAT32(LSHIFT(int32(input[k]), 12), out32_Q12)

		out32 = RSHIFT_ROUND(out32_Q12, 12)

		output[k] = int16(SAT16(out32))

		state[0] = input[k]
	}
}

func lpcAnalysisFilter16(input, coefficients, state, output []int16, length int32) {
	input = input[:length]
	output = output[:length]
	coefficients = coefficients[:16]
	state = state[:16]
	for k := range input {
		SA := state[0]
		var SB int16
		var prediction int32
		SB = state[1]
		state[1] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[0]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[1]))
		SA = state[2]
		state[2] = SB
		SB = state[3]
		state[3] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[2]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[3]))
		SA = state[4]
		state[4] = SB
		SB = state[5]
		state[5] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[4]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[5]))
		SA = state[6]
		state[6] = SB
		SB = state[7]
		state[7] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[6]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[7]))
		SA = state[8]
		state[8] = SB
		SB = state[9]
		state[9] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[8]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[9]))
		SA = state[10]
		state[10] = SB
		SB = state[11]
		state[11] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[10]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[11]))
		SA = state[12]
		state[12] = SB
		SB = state[13]
		state[13] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[12]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[13]))
		SA = state[14]
		state[14] = SB
		SB = state[15]
		state[15] = SA
		prediction = SMLABB(prediction, int32(SA), int32(coefficients[14]))
		prediction = SMLABB(prediction, int32(SB), int32(coefficients[15]))
		residual := SUB_SAT32(LSHIFT(int32(input[k]), 12), prediction)
		output[k] = int16(SAT16(RSHIFT_ROUND(residual, 12)))
		// Re-read after writing output to preserve in-place filtering semantics.
		state[0] = input[k]
	}
}
