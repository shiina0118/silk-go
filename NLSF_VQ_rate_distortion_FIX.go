package silk

func NLSF_VQ_rate_distortion_FIX(
	pRD_Q20 *slice[int32],
	psNLSF_CBS *NLSF_CBS,
	in_Q15, w_Q6, rate_acc_Q5 *slice[int32],
	mu_Q15, N, LPC_order int32) {

	var i, n int32

	NLSF_VQ_sum_error_FIX(pRD_Q20, in_Q15, w_Q6, psNLSF_CBS.CB_NLSF_Q15,
		N, psNLSF_CBS.nVectors, LPC_order)

	rateDistortion := pRD_Q20.From()
	accumulatedRates := rate_acc_Q5.From()
	codebookRates := psNLSF_CBS.Rates_Q5.From()
	for n = 0; n < N; n++ {
		for i = 0; i < psNLSF_CBS.nVectors; i++ {
			offset := int(n*psNLSF_CBS.nVectors + i)
			rateDistortion[offset] = SMLABB(rateDistortion[offset], accumulatedRates[n]+int32(codebookRates[i]), mu_Q15)
		}
	}
}
