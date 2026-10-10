package silk

const ORDER_FIR = 6

func resampler_down3(S *slice[int32], out, in *slice[int16], inLen int32) {
	var buf [RESAMPLER_MAX_BATCH_SIZE_IN + ORDER_FIR]int32
	state, input, output := S.From(), in.From(), out.From()
	copy(buf[:ORDER_FIR], state[:ORDER_FIR])
	coefs := Resampler_1_3_COEFS_LQ
	inputOffset, outputOffset := 0, 0

	for {
		nSamplesIn := min(inLen, RESAMPLER_MAX_BATCH_SIZE_IN)
		resamplerPrivateAR2(state[ORDER_FIR:], buf[ORDER_FIR:], input[inputOffset:], coefs, nSamplesIn)

		for bufferOffset, counter := 0, nSamplesIn; counter > 2; bufferOffset, counter = bufferOffset+3, counter-3 {
			res_Q6 := SMULWB(ADD32(buf[bufferOffset], buf[bufferOffset+5]), int32(coefs[2]))
			res_Q6 = SMLAWB(res_Q6, ADD32(buf[bufferOffset+1], buf[bufferOffset+4]), int32(coefs[3]))
			res_Q6 = SMLAWB(res_Q6, ADD32(buf[bufferOffset+2], buf[bufferOffset+3]), int32(coefs[4]))

			output[outputOffset] = int16(SAT16(RSHIFT_ROUND(res_Q6, 6)))
			outputOffset++
		}

		inputOffset += int(nSamplesIn)
		inLen -= nSamplesIn

		if inLen > 0 {
			copy(buf[:ORDER_FIR], buf[int(nSamplesIn):int(nSamplesIn)+ORDER_FIR])
		} else {
			copy(state[:ORDER_FIR], buf[int(nSamplesIn):int(nSamplesIn)+ORDER_FIR])
			break
		}
	}
}
