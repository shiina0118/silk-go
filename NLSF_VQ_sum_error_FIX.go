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
		inputVector := input[n*LPC_order : (n+1)*LPC_order]
		errors := err[n*K : (n+1)*K]
		switch LPC_order {
		case 10:
			nlsfSumError10(errors, inputVector, &Wcpy_Q6, codebook, K)
			continue
		case 16:
			nlsfSumError16(errors, inputVector, &Wcpy_Q6, codebook, K)
			continue
		}
		for i = 0; i < K; i++ {
			codebookVector := codebook[i*LPC_order : (i+1)*LPC_order]
			sum_error = 0
			for m = 0; m < LPC_order; m += 2 {
				Wtmp_Q6 = Wcpy_Q6[RSHIFT(m, 1)]
				_ = inputVector[m+1]
				_ = codebookVector[m+1]

				diff_Q15 = inputVector[m] - int32(codebookVector[m])
				sum_error = SMLAWB(sum_error, SMULBB(diff_Q15, diff_Q15), Wtmp_Q6)

				diff_Q15 = inputVector[m+1] - int32(codebookVector[m+1])
				sum_error = SMLAWT(sum_error, SMULBB(diff_Q15, diff_Q15), Wtmp_Q6)
			}

			errors[i] = sum_error
		}
	}
}

// Each weighted term is rounded before summation, exactly as in SMLAWB/T.
// The independent pair sums combine using the same modulo-2^32 addition.
func nlsfPairError(in0, in1 int32, cb0, cb1 int16, weight int32) int32 {
	diff0 := in0 - int32(cb0)
	diff1 := in1 - int32(cb1)
	return SMULWB(SMULBB(diff0, diff0), weight) + SMULWT(SMULBB(diff1, diff1), weight)
}

func nlsfSumError10(errors, input []int32, weights *[MAX_LPC_ORDER / 2]int32, codebook []int16, count int32) {
	_ = input[9]
	in0 := input[0]
	in1 := input[1]
	in2 := input[2]
	in3 := input[3]
	in4 := input[4]
	in5 := input[5]
	in6 := input[6]
	in7 := input[7]
	in8 := input[8]
	in9 := input[9]
	w0 := weights[0]
	w1 := weights[1]
	w2 := weights[2]
	w3 := weights[3]
	w4 := weights[4]
	for i := int32(0); i < count; i++ {
		cb := codebook[i*10 : (i+1)*10]
		e0 := nlsfPairError(in0, in1, cb[0], cb[1], w0)
		e1 := nlsfPairError(in2, in3, cb[2], cb[3], w1)
		e2 := nlsfPairError(in4, in5, cb[4], cb[5], w2)
		e3 := nlsfPairError(in6, in7, cb[6], cb[7], w3)
		e4 := nlsfPairError(in8, in9, cb[8], cb[9], w4)
		errors[i] = e0 + e1 + e2 + e3 + e4
	}
}

func nlsfSumError16(errors, input []int32, weights *[MAX_LPC_ORDER / 2]int32, codebook []int16, count int32) {
	_ = input[15]
	in0 := input[0]
	in1 := input[1]
	in2 := input[2]
	in3 := input[3]
	in4 := input[4]
	in5 := input[5]
	in6 := input[6]
	in7 := input[7]
	in8 := input[8]
	in9 := input[9]
	in10 := input[10]
	in11 := input[11]
	in12 := input[12]
	in13 := input[13]
	in14 := input[14]
	in15 := input[15]
	w0 := weights[0]
	w1 := weights[1]
	w2 := weights[2]
	w3 := weights[3]
	w4 := weights[4]
	w5 := weights[5]
	w6 := weights[6]
	w7 := weights[7]
	for i := int32(0); i < count; i++ {
		cb := codebook[i*16 : (i+1)*16]
		e0 := nlsfPairError(in0, in1, cb[0], cb[1], w0)
		e1 := nlsfPairError(in2, in3, cb[2], cb[3], w1)
		e2 := nlsfPairError(in4, in5, cb[4], cb[5], w2)
		e3 := nlsfPairError(in6, in7, cb[6], cb[7], w3)
		e4 := nlsfPairError(in8, in9, cb[8], cb[9], w4)
		e5 := nlsfPairError(in10, in11, cb[10], cb[11], w5)
		e6 := nlsfPairError(in12, in13, cb[12], cb[13], w6)
		e7 := nlsfPairError(in14, in15, cb[14], cb[15], w7)
		errors[i] = e0 + e1 + e2 + e3 + e4 + e5 + e6 + e7
	}
}
