package silk

func resampler_private_AR2(S, out_Q8 *slice[int32], in, A_Q14 *slice[int16], inLen int32) {
	resamplerPrivateAR2(S.From(), out_Q8.From(), in.From(), A_Q14.From(), inLen)
}

// The downsamplers already have contiguous buffers, so they can share this
// kernel without allocating slice offset wrappers for each sample.
func resamplerPrivateAR2(state, output []int32, input, coefs []int16, inLen int32) {
	for k := 0; k < int(inLen); k++ {
		out32 := ADD_LSHIFT32(state[0], int32(input[k]), 8)
		output[k] = out32
		out32 = LSHIFT(out32, 2)
		state[0] = SMLAWB(state[1], out32, int32(coefs[0]))
		state[1] = SMULWB(out32, int32(coefs[1]))
	}
}
