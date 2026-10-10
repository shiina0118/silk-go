package silk

import "math"

func NLSF_MSVQ_encode_FIX(NLSFIndices, pNLSF_Q15 *slice[int32], psNLSF_CB *NLSF_CB_struct,
	pNLSF_q_Q15_prev, pW_Q6 *slice[int32], NLSF_mu_Q15, NLSF_mu_fluc_red_Q16, NLSF_MSVQ_Survivors, LPC_order, deactivate_fluc_red int32) {
	var (
		i, s, k, cur_survivors, prev_survivors, min_survivors, input_index, cb_index, bestIndex int32
		rateDistThreshold_Q18                                                                   int32
		se_Q15, wsse_Q20, bestRateDist_Q20                                                      int32
		pRateDist_Q18Buffer                                                                     [NLSF_MSVQ_TREE_SEARCH_MAX_VECTORS_EVALUATED]int32
		pRate_Q5Buffer                                                                          [MAX_NLSF_MSVQ_SURVIVORS]int32
		pRate_new_Q5Buffer                                                                      [MAX_NLSF_MSVQ_SURVIVORS]int32
		pTempIndicesBuffer                                                                      [MAX_NLSF_MSVQ_SURVIVORS]int32
		pPathBuffer                                                                             [MAX_NLSF_MSVQ_SURVIVORS * NLSF_MSVQ_MAX_CB_STAGES]int32
		pPath_newBuffer                                                                         [MAX_NLSF_MSVQ_SURVIVORS * NLSF_MSVQ_MAX_CB_STAGES]int32
		pRes_Q15Buffer                                                                          [MAX_NLSF_MSVQ_SURVIVORS * MAX_LPC_ORDER]int32
		pRes_new_Q15Buffer                                                                      [MAX_NLSF_MSVQ_SURVIVORS * MAX_LPC_ORDER]int32
		pCurrentCBStage                                                                         *NLSF_CBS
	)
	pRateDist_Q18 := &slice[int32]{arr: pRateDist_Q18Buffer[:], size: 4}
	pRate_Q5 := &slice[int32]{arr: pRate_Q5Buffer[:], size: 4}
	pRate_new_Q5 := &slice[int32]{arr: pRate_new_Q5Buffer[:], size: 4}
	pTempIndices := &slice[int32]{arr: pTempIndicesBuffer[:], size: 4}
	pPath := &slice[int32]{arr: pPathBuffer[:], size: 4}
	pPath_new := &slice[int32]{arr: pPath_newBuffer[:], size: 4}
	pRes_Q15 := &slice[int32]{arr: pRes_Q15Buffer[:], size: 4}
	pRes_new_Q15 := &slice[int32]{arr: pRes_new_Q15Buffer[:], size: 4}
	residuals := pRes_Q15.From()
	newResiduals := pRes_new_Q15.From()
	paths := pPath.From()
	newPaths := pPath_new.From()
	rates := pRate_Q5.From()
	newRates := pRate_new_Q5.From()
	temporaryIndices := pTempIndices.From()
	rateDistortion := pRateDist_Q18.From()
	nlsf := pNLSF_Q15.From()
	previousNLSF := pNLSF_q_Q15_prev.From()
	weights := pW_Q6.From()

	memset(pRate_Q5, 0, int(NLSF_MSVQ_Survivors))

	for i = 0; i < LPC_order; i++ {
		residuals[i] = nlsf[int(i)]
	}

	prev_survivors = 1

	min_survivors = NLSF_MSVQ_Survivors / 2

	for s = 0; s < psNLSF_CB.nStages; s++ {

		pCurrentCBStage = &psNLSF_CB.CBStages[s]

		cur_survivors = min(NLSF_MSVQ_Survivors, SMULBB(prev_survivors, pCurrentCBStage.nVectors))

		NLSF_VQ_rate_distortion_FIX(pRateDist_Q18, pCurrentCBStage, pRes_Q15, pW_Q6,
			pRate_Q5, NLSF_mu_Q15, prev_survivors, LPC_order)

		insertion_sort_increasing(pRateDist_Q18, pTempIndices,
			prev_survivors*pCurrentCBStage.nVectors, cur_survivors)

		if rateDistortion[0] < math.MaxInt32/MAX_NLSF_MSVQ_SURVIVORS {
			rateDistThreshold_Q18 = SMLAWB(rateDistortion[0],
				MUL(NLSF_MSVQ_Survivors, rateDistortion[0]), FIX_CONST(NLSF_MSVQ_SURV_MAX_REL_RD, 16))
			for rateDistortion[int(cur_survivors-1)] > rateDistThreshold_Q18 && cur_survivors > min_survivors {
				cur_survivors--
			}
		}
		codebook := pCurrentCBStage.CB_NLSF_Q15.From()
		codebookRates := pCurrentCBStage.Rates_Q5.From()
		for k = 0; k < cur_survivors; k++ {
			if s > 0 {
				if pCurrentCBStage.nVectors == 8 {
					input_index = RSHIFT(temporaryIndices[int(k)], 3)
					cb_index = temporaryIndices[int(k)] & 7
				} else {
					input_index = DIV32_16(temporaryIndices[int(k)], int16(pCurrentCBStage.nVectors))
					cb_index = temporaryIndices[int(k)] - SMULBB(input_index, pCurrentCBStage.nVectors)
				}
			} else {
				input_index = 0
				cb_index = temporaryIndices[k]
			}

			residualOffset := input_index * LPC_order
			codebookOffset := cb_index * LPC_order
			newResidualOffset := k * LPC_order
			for i = 0; i < LPC_order; i++ {
				newResiduals[newResidualOffset+i] = residuals[residualOffset+i] - int32(codebook[codebookOffset+i])
			}

			newRates[k] = rates[input_index] + int32(codebookRates[int(cb_index)])

			pathOffset := input_index * psNLSF_CB.nStages
			newPathOffset := k * psNLSF_CB.nStages
			for i = 0; i < s; i++ {
				newPaths[newPathOffset+i] = paths[pathOffset+i]
			}
			newPaths[newPathOffset+s] = cb_index
		}

		if s < psNLSF_CB.nStages-1 {
			pRes_new_Q15.copy(pRes_Q15, int(SMULBB(cur_survivors, LPC_order)))
			pRate_new_Q5.copy(pRate_Q5, int(cur_survivors))
			pPath_new.copy(pPath, int(SMULBB(cur_survivors, psNLSF_CB.nStages)))
		}

		prev_survivors = cur_survivors
	}

	bestIndex = 0

	if deactivate_fluc_red != 1 {

		bestRateDist_Q20 = math.MaxInt32
		for s = 0; s < cur_survivors; s++ {
			NLSF_MSVQ_decode(pNLSF_Q15, psNLSF_CB, pPath_new.off(int(SMULBB(s, psNLSF_CB.nStages))), LPC_order)

			wsse_Q20 = 0
			for i = 0; i < LPC_order; i += 2 {
				se_Q15 = nlsf[int(i)] - previousNLSF[int(i)]
				wsse_Q20 = SMLAWB(wsse_Q20, SMULBB(se_Q15, se_Q15), weights[int(i)])

				se_Q15 = nlsf[int(i+1)] - previousNLSF[int(i+1)]
				wsse_Q20 = SMLAWB(wsse_Q20, SMULBB(se_Q15, se_Q15), weights[int(i+1)])
			}

			wsse_Q20 = ADD_POS_SAT32(rateDistortion[int(s)], SMULWB(wsse_Q20, NLSF_mu_fluc_red_Q16))

			if wsse_Q20 < bestRateDist_Q20 {
				bestRateDist_Q20 = wsse_Q20
				bestIndex = s
			}
		}
	}

	bestPathOffset := int(bestIndex * psNLSF_CB.nStages)
	copy(NLSFIndices.From()[:psNLSF_CB.nStages], newPaths[bestPathOffset:bestPathOffset+int(psNLSF_CB.nStages)])

	NLSF_MSVQ_decode(pNLSF_Q15, psNLSF_CB, NLSFIndices, LPC_order)
}
