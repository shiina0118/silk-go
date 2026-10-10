package silk

func NLSF_MSVQ_decode(pNLSF_Q15 *slice[int32], psNLSF_CB *NLSF_CB_struct, NLSFIndices *slice[int32], LPC_order int32) {
	var (
		pCB_element []int16
		s, i        int32
	)

	nlsf := pNLSF_Q15.From()
	indices := NLSFIndices.From()
	pCB_element = psNLSF_CB.CBStages[0].CB_NLSF_Q15.From()[int(MUL(indices[0], LPC_order)):]

	for i = 0; i < LPC_order; i++ {
		nlsf[int(i)] = int32(pCB_element[int(i)])
	}

	for s = 1; s < psNLSF_CB.nStages; s++ {

		if LPC_order == 16 {
			pCB_element = psNLSF_CB.CBStages[s].CB_NLSF_Q15.From()[int(LSHIFT(indices[int(s)], 4)):]

			nlsf[0] += int32(pCB_element[0])
			nlsf[1] += int32(pCB_element[1])
			nlsf[2] += int32(pCB_element[2])
			nlsf[3] += int32(pCB_element[3])
			nlsf[4] += int32(pCB_element[4])
			nlsf[5] += int32(pCB_element[5])
			nlsf[6] += int32(pCB_element[6])
			nlsf[7] += int32(pCB_element[7])
			nlsf[8] += int32(pCB_element[8])
			nlsf[9] += int32(pCB_element[9])
			nlsf[10] += int32(pCB_element[10])
			nlsf[11] += int32(pCB_element[11])
			nlsf[12] += int32(pCB_element[12])
			nlsf[13] += int32(pCB_element[13])
			nlsf[14] += int32(pCB_element[14])
			nlsf[15] += int32(pCB_element[15])
		} else {
			pCB_element = psNLSF_CB.CBStages[s].CB_NLSF_Q15.From()[int(SMULBB(indices[int(s)], LPC_order)):]

			for i = 0; i < LPC_order; i++ {
				nlsf[int(i)] += int32(pCB_element[int(i)])
			}
		}
	}

	NLSF_stabilize(pNLSF_Q15, psNLSF_CB.NDeltaMin_Q15, LPC_order)
}
