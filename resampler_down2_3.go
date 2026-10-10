package silk

const DOWN23_ORDER_FIR = 4

func resampler_down2_3(S *slice[int32], out, in *slice[int16], inLen int32) {
	var buf [RESAMPLER_MAX_BATCH_SIZE_IN + DOWN23_ORDER_FIR]int32
	state, input, output := S.From(), in.From(), out.From()
	copy(buf[:DOWN23_ORDER_FIR], state[:DOWN23_ORDER_FIR])
	coefs := Resampler_2_3_COEFS_LQ
	inputOffset, outputOffset := 0, 0

	for {
		nSamplesIn := min(inLen, RESAMPLER_MAX_BATCH_SIZE_IN)
		resamplerPrivateAR2(state[DOWN23_ORDER_FIR:], buf[DOWN23_ORDER_FIR:], input[inputOffset:], coefs, nSamplesIn)

		for bufferOffset, counter := 0, nSamplesIn; counter > 2; bufferOffset, counter = bufferOffset+3, counter-3 {
			res_Q6 := SMULWB(buf[bufferOffset], int32(coefs[2]))
			res_Q6 = SMLAWB(res_Q6, buf[bufferOffset+1], int32(coefs[3]))
			res_Q6 = SMLAWB(res_Q6, buf[bufferOffset+2], int32(coefs[5]))
			res_Q6 = SMLAWB(res_Q6, buf[bufferOffset+3], int32(coefs[4]))

			output[outputOffset] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			outputOffset++

			res_Q6 = SMULWB(buf[bufferOffset+1], int32(coefs[4]))
			res_Q6 = SMLAWB(res_Q6, buf[bufferOffset+2], int32(coefs[5]))
			res_Q6 = SMLAWB(res_Q6, buf[bufferOffset+3], int32(coefs[3]))
			res_Q6 = SMLAWB(res_Q6, buf[bufferOffset+4], int32(coefs[2]))

			output[outputOffset] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			outputOffset++
		}

		inputOffset += int(nSamplesIn)
		inLen -= nSamplesIn

		if inLen > 0 {
			copy(buf[:DOWN23_ORDER_FIR], buf[int(nSamplesIn):int(nSamplesIn)+DOWN23_ORDER_FIR])
		} else {
			copy(state[:DOWN23_ORDER_FIR], buf[int(nSamplesIn):int(nSamplesIn)+DOWN23_ORDER_FIR])
			break
		}
	}
}
