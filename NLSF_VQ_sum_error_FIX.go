package silk

func NLSF_VQ_sum_error_FIX(
	err_Q20, in_Q15, w_Q6 *slice[int32],
	pCB_Q15 *slice[int16],
	N, K, LPC_order int32) {
	var (
		i, n, m                      int32
		diff_Q15, sum_error, Wtmp_Q6 int32
		Wcpy_Q6                      [MAX_LPC_ORDER / 2]int32
	)
	err := err_Q20.From()
	input := in_Q15.From()
	weights := w_Q6.From()
	codebook := pCB_Q15.From()

	for m = 0; m < RSHIFT(LPC_order, 1); m++ {
		Wcpy_Q6[m] = weights[2*m] | LSHIFT(weights[2*m+1], 16)
	}

	for n = 0; n < N; n++ {
		for i = 0; i < K; i++ {
			sum_error = 0
			for m = 0; m < LPC_order; m += 2 {
				Wtmp_Q6 = Wcpy_Q6[RSHIFT(m, 1)]
				inputOffset := int(n*LPC_order + m)
				codebookOffset := int(i*LPC_order + m)

				diff_Q15 = input[inputOffset] - int32(codebook[codebookOffset])
				sum_error = SMLAWB(sum_error, SMULBB(diff_Q15, diff_Q15), Wtmp_Q6)

				diff_Q15 = input[inputOffset+1] - int32(codebook[codebookOffset+1])
				sum_error = SMLAWT(sum_error, SMULBB(diff_Q15, diff_Q15), Wtmp_Q6)
			}

			err[int(n*K+i)] = sum_error
		}
	}
}
