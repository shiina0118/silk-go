package silk

import (
	"math"
)

type NSQ_del_dec_struct struct {
	RandState [DECISION_DELAY]int32
	Q_Q10     [DECISION_DELAY]int32
	Xq_Q10    [DECISION_DELAY]int32
	Pred_Q16  [DECISION_DELAY]int32
	Shape_Q10 [DECISION_DELAY]int32
	Gain_Q16  [DECISION_DELAY]int32
	sAR2_Q14  [MAX_SHAPE_LPC_ORDER]int32
	sLPC_Q14  [MAX_FRAME_LENGTH/NB_SUBFR + NSQ_LPC_BUF_LENGTH]int32
	LF_AR_Q12 int32
	Seed      int32
	SeedInit  int32
	RD_Q10    int32
}

// Each quantizer owns its scratch state. It is reset before every frame, just
// like the temporary zero-filled arrays used by the original implementation.
type nsqDelDecWorkspace struct {
	states   [MAX_DEL_DEC_STATES]NSQ_del_dec_struct
	sLTP_Q16 [2 * MAX_FRAME_LENGTH]int32
	sLTP     [2 * MAX_FRAME_LENGTH]int16
	filter   [MAX_LPC_ORDER]int32
	xScaled  [MAX_FRAME_LENGTH / NB_SUBFR]int32
}

type NSQ_sample_struct struct {
	Q_Q10        int32
	RD_Q10       int32
	xq_Q14       int32
	LF_AR_Q12    int32
	sLTP_shp_Q10 int32
	LPC_exc_Q16  int32
}

func (n *NSQ_sample_struct) copy(dest *NSQ_sample_struct) {
	dest.Q_Q10 = n.Q_Q10
	dest.RD_Q10 = n.RD_Q10
	dest.xq_Q14 = n.xq_Q14
	dest.LF_AR_Q12 = n.LF_AR_Q12
	dest.sLTP_shp_Q10 = n.sLTP_shp_Q10
	dest.LPC_exc_Q16 = n.LPC_exc_Q16
}

func NSQ_del_dec(
	psEncC *encoder_state,
	psEncCtrlC *encoder_control,
	NSQ *nsq_state,
	x *slice[int16],
	q *slice[int8],
	LSFInterpFactor_Q2 int32,
	PredCoef_Q12 [2]*slice[int16], LTPCoef_Q14, AR2_Q13 *slice[int16],
	HarmShapeGain_Q14 *slice[int32],
	Tilt_Q14 *slice[int32],
	LF_shp_Q14 *slice[int32],
	Gains_Q16 *slice[int32],
	Lambda_Q10, LTP_scale_Q14 int32) {
	if NSQ.delDecWorkspace == nil {
		NSQ.delDecWorkspace = new(nsqDelDecWorkspace)
	}
	work := NSQ.delDecWorkspace
	*work = nsqDelDecWorkspace{}
	var (
		i, k, lag, start_idx, LSF_interpolation_flag, Winner_ind, subfr int32
		last_smple_idx, smpl_buf_idx, decisionDelay, subfr_length       int32
		A_Q12, B_Q14, AR_shp_Q13                                        *slice[int16]
		pxq                                                             *slice[int16]
		sLTP_Q16                                                        = &slice[int32]{arr: work.sLTP_Q16[:], size: 4}
		sLTP                                                            = &slice[int16]{arr: work.sLTP[:], size: 2}
		HarmShapeFIRPacked_Q14, offset_Q10                              int32
		FiltState                                                       = &slice[int32]{arr: work.filter[:], size: 4}
		RDmin_Q10                                                       int32
		x_sc_Q10                                                        = &slice[int32]{arr: work.xScaled[:], size: 4}
		psDelDec                                                        = &work.states
		psDD                                                            *NSQ_del_dec_struct
	)

	subfr_length = psEncC.frame_length / NB_SUBFR

	lag = NSQ.lagPrev

	for k = 0; k < psEncC.nStatesDelayedDecision; k++ {
		psDD = &psDelDec[k]
		psDD.Seed = (k + psEncCtrlC.Seed) & 3
		psDD.SeedInit = psDD.Seed
		psDD.RD_Q10 = 0
		psDD.LF_AR_Q12 = NSQ.sLF_AR_shp_Q12
		psDD.Shape_Q10[0] = NSQ.sLTP_shp_Q10.idx(int(psEncC.frame_length - 1))
		copy(psDD.sLPC_Q14[:NSQ_LPC_BUF_LENGTH], NSQ.sLPC_Q14.From())
		copy(psDD.sAR2_Q14[:], NSQ.sAR2_Q14.From())
	}

	offset_Q10 = int32(Quantization_Offsets_Q10[psEncCtrlC.sigtype][psEncCtrlC.QuantOffsetType])
	smpl_buf_idx = 0

	decisionDelay = min(DECISION_DELAY, subfr_length)

	if psEncCtrlC.sigtype == SIG_TYPE_VOICED {
		for k = 0; k < NB_SUBFR; k++ {
			decisionDelay = min(decisionDelay, psEncCtrlC.pitchL.idx(int(k))-LTP_ORDER/2-1)
		}
	} else {
		if lag > 0 {
			decisionDelay = min(decisionDelay, lag-LTP_ORDER/2-1)
		}
	}

	if LSFInterpFactor_Q2 == (1 << 2) {
		LSF_interpolation_flag = 0
	} else {
		LSF_interpolation_flag = 1
	}

	pxq = NSQ.xq.off(int(psEncC.frame_length))
	NSQ.sLTP_shp_buf_idx = psEncC.frame_length
	NSQ.sLTP_buf_idx = psEncC.frame_length
	subfr = 0
	for k = 0; k < NB_SUBFR; k++ {
		A_Q12 = PredCoef_Q12[(k>>1)|(1-LSF_interpolation_flag)]
		B_Q14 = LTPCoef_Q14.off(int(k * LTP_ORDER))
		AR_shp_Q13 = AR2_Q13.off(int(k * MAX_SHAPE_LPC_ORDER))

		HarmShapeFIRPacked_Q14 = RSHIFT(HarmShapeGain_Q14.idx(int(k)), 2)
		HarmShapeFIRPacked_Q14 |= LSHIFT(RSHIFT(HarmShapeGain_Q14.idx(int(k)), 1), 16)

		NSQ.rewhite_flag = 0
		if psEncCtrlC.sigtype == SIG_TYPE_VOICED {
			lag = psEncCtrlC.pitchL.idx(int(k))

			if (k & (3 - LSHIFT(LSF_interpolation_flag, 1))) == 0 {
				if k == 2 {
					RDmin_Q10 = psDelDec[0].RD_Q10
					Winner_ind = 0
					for i = 1; i < psEncC.nStatesDelayedDecision; i++ {
						if psDelDec[i].RD_Q10 < RDmin_Q10 {
							RDmin_Q10 = psDelDec[i].RD_Q10
							Winner_ind = i
						}
					}
					for i = 0; i < psEncC.nStatesDelayedDecision; i++ {
						if i != Winner_ind {
							psDelDec[i].RD_Q10 += math.MaxInt32 >> 4
						}
					}

					psDD = &psDelDec[Winner_ind]
					last_smple_idx = smpl_buf_idx + decisionDelay
					for i = 0; i < decisionDelay; i++ {
						last_smple_idx = (last_smple_idx - 1) & DECISION_DELAY_MASK
						*q.ptr(int(i - decisionDelay)) = int8(RSHIFT(psDD.Q_Q10[last_smple_idx], 10))
						*pxq.ptr(int(i - decisionDelay)) = int16(SAT16(RSHIFT_ROUND(
							SMULWW(psDD.Xq_Q10[last_smple_idx],
								psDD.Gain_Q16[last_smple_idx]), 10)))
						*NSQ.sLTP_shp_Q10.ptr(int(NSQ.sLTP_shp_buf_idx - decisionDelay + i)) = psDD.Shape_Q10[last_smple_idx]
					}

					subfr = 0
				}

				start_idx = psEncC.frame_length - lag - psEncC.predictLPCOrder - LTP_ORDER/2

				memset(FiltState, 0, int(psEncC.predictLPCOrder))
				MA_Prediction(NSQ.xq.off(int(start_idx+k*psEncC.subfr_length)),
					A_Q12, FiltState, sLTP.off(int(start_idx)), psEncC.frame_length-start_idx, psEncC.predictLPCOrder)

				NSQ.sLTP_buf_idx = psEncC.frame_length
				NSQ.rewhite_flag = 1
			}
		}

		nsq_del_dec_scale_states(NSQ, psDelDec[:], x, x_sc_Q10,
			subfr_length, sLTP, sLTP_Q16, k, psEncC.nStatesDelayedDecision, smpl_buf_idx,
			LTP_scale_Q14, Gains_Q16, psEncCtrlC.pitchL)

		noise_shape_quantizer_del_dec(NSQ, psDelDec[:], psEncCtrlC.sigtype, x_sc_Q10, q, pxq, sLTP_Q16,
			A_Q12, B_Q14, AR_shp_Q13, lag, HarmShapeFIRPacked_Q14, Tilt_Q14.idx(int(k)), LF_shp_Q14.idx(int(k)), Gains_Q16.idx(int(k)),
			Lambda_Q10, offset_Q10, psEncC.subfr_length, subfr, psEncC.shapingLPCOrder, psEncC.predictLPCOrder,
			psEncC.warping_Q16, psEncC.nStatesDelayedDecision, &smpl_buf_idx, decisionDelay)
		subfr++

		x = x.off(int(psEncC.subfr_length))
		q = q.off(int(psEncC.subfr_length))
		pxq = pxq.off(int(psEncC.subfr_length))
	}

	RDmin_Q10 = psDelDec[0].RD_Q10
	Winner_ind = 0
	for k = 1; k < psEncC.nStatesDelayedDecision; k++ {
		if psDelDec[k].RD_Q10 < RDmin_Q10 {
			RDmin_Q10 = psDelDec[k].RD_Q10
			Winner_ind = k
		}
	}

	psDD = &psDelDec[Winner_ind]
	psEncCtrlC.Seed = psDD.SeedInit
	last_smple_idx = smpl_buf_idx + decisionDelay
	for i = 0; i < decisionDelay; i++ {
		last_smple_idx = (last_smple_idx - 1) & DECISION_DELAY_MASK
		*q.ptr(int(i - decisionDelay)) = int8(RSHIFT(psDD.Q_Q10[last_smple_idx], 10))
		*pxq.ptr(int(i - decisionDelay)) = int16(SAT16(RSHIFT_ROUND(
			SMULWW(psDD.Xq_Q10[last_smple_idx], psDD.Gain_Q16[last_smple_idx]), 10)))
		*NSQ.sLTP_shp_Q10.ptr(int(NSQ.sLTP_shp_buf_idx - decisionDelay + i)) = psDD.Shape_Q10[last_smple_idx]
		*sLTP_Q16.ptr(int(NSQ.sLTP_buf_idx - decisionDelay + i)) = psDD.Pred_Q16[last_smple_idx]
	}

	copy(NSQ.sLPC_Q14.From()[:NSQ_LPC_BUF_LENGTH], psDD.sLPC_Q14[psEncC.subfr_length:psEncC.subfr_length+NSQ_LPC_BUF_LENGTH])
	copy(NSQ.sAR2_Q14.From()[:MAX_SHAPE_LPC_ORDER], psDD.sAR2_Q14[:])

	NSQ.sLF_AR_shp_Q12 = psDD.LF_AR_Q12
	NSQ.lagPrev = psEncCtrlC.pitchL.idx(NB_SUBFR - 1)

	NSQ.xq.off(int(psEncC.frame_length)).copy(NSQ.xq, int(psEncC.frame_length))
	NSQ.sLTP_shp_Q10.off(int(psEncC.frame_length)).copy(NSQ.sLTP_shp_Q10, int(psEncC.frame_length))
}

func noise_shape_quantizer_del_dec(
	NSQ *nsq_state,
	psDelDec []NSQ_del_dec_struct,
	sigtype int32,
	x_Q10 *slice[int32],
	q *slice[int8],
	xq *slice[int16],
	sLTP_Q16 *slice[int32],
	a_Q12 *slice[int16],
	b_Q14 *slice[int16],
	AR_shp_Q13 *slice[int16],
	lag int32,
	HarmShapeFIRPacked_Q14 int32,
	Tilt_Q14 int32,
	LF_shp_Q14 int32,
	Gain_Q16 int32,
	Lambda_Q10 int32,
	offset_Q10 int32,
	length int32,
	subfr int32,
	shapingLPCOrder int32,
	predictLPCOrder int32,
	warping_Q16 int32,
	nStatesDelayedDecision int32,
	smpl_buf_idx *int32,
	decisionDelay int32) {
	var (
		i, j, k, Winner_ind, RDmin_ind, RDmax_ind, last_smple_idx       int32
		Winner_rand_state                                               int32
		LTP_pred_Q14, LPC_pred_Q10, n_AR_Q10, n_LTP_Q14                 int32
		n_LF_Q10, r_Q10, rr_Q20, rd1_Q10, rd2_Q10, RDmin_Q10, RDmax_Q10 int32
		q1_Q10, q2_Q10, dither, exc_Q10, LPC_exc_Q10, xq_Q10            int32
		tmp1, tmp2, sLF_AR_shp_Q10                                      int32
		predLagIndex, shapeLagIndex, lpcIndex                           int
		psSampleState                                                   [MAX_DEL_DEC_STATES][2]NSQ_sample_struct
		psDD                                                            *NSQ_del_dec_struct
		psSS                                                            *[2]NSQ_sample_struct
		a_Q12_tmp                                                       [MAX_LPC_ORDER / 2]int32
		aWide                                                           [MAX_LPC_ORDER]int64
		shapeWide                                                       [MAX_SHAPE_LPC_ORDER]int64
		Atmp                                                            int32
	)

	aCoefficients := a_Q12.From()
	bCoefficients := b_Q14.From()[:LTP_ORDER]
	shapeCoefficients := AR_shp_Q13.From()[:shapingLPCOrder]
	input := x_Q10.From()[:length]
	pulseData, pulseOffset := q.arr, q.offset
	outputData, outputOffset := xq.arr, xq.offset
	for coefficientIndex := 0; coefficientIndex < int(predictLPCOrder); coefficientIndex += 2 {
		a_Q12_tmp[coefficientIndex/2] = int32(uint16(aCoefficients[coefficientIndex])) |
			int32(aCoefficients[coefficientIndex+1])<<16
		aWide[coefficientIndex] = int64(aCoefficients[coefficientIndex])
		aWide[coefficientIndex+1] = int64(aCoefficients[coefficientIndex+1])
	}
	if shapingLPCOrder == 16 {
		for coefficientIndex := range shapeCoefficients {
			shapeWide[coefficientIndex] = int64(shapeCoefficients[coefficientIndex])
		}
	}
	signedWarp := int64(int16(warping_Q16))

	shapeHistory := NSQ.sLTP_shp_Q10.arr
	shapeLagIndex = NSQ.sLTP_shp_Q10.offset + int(NSQ.sLTP_shp_buf_idx-lag+HARM_SHAPE_FIR_TAPS/2)
	predictionHistory := sLTP_Q16.arr
	predLagIndex = sLTP_Q16.offset + int(NSQ.sLTP_buf_idx-lag+LTP_ORDER/2)

	for i = 0; i < length; i++ {

		if sigtype == SIG_TYPE_VOICED {
			LTP_pred_Q14 = SMULWB(predictionHistory[predLagIndex], int32(bCoefficients[0]))
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, predictionHistory[predLagIndex-1], int32(bCoefficients[1]))
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, predictionHistory[predLagIndex-2], int32(bCoefficients[2]))
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, predictionHistory[predLagIndex-3], int32(bCoefficients[3]))
			LTP_pred_Q14 = SMLAWB(LTP_pred_Q14, predictionHistory[predLagIndex-4], int32(bCoefficients[4]))
			predLagIndex++
		} else {
			LTP_pred_Q14 = 0
		}

		if lag > 0 {
			n_LTP_Q14 = SMULWB(ADD32(shapeHistory[shapeLagIndex], shapeHistory[shapeLagIndex-2]), HarmShapeFIRPacked_Q14)
			n_LTP_Q14 = SMLAWT(n_LTP_Q14, shapeHistory[shapeLagIndex-1], HarmShapeFIRPacked_Q14)
			n_LTP_Q14 = LSHIFT(n_LTP_Q14, 6)
			shapeLagIndex++
		} else {
			n_LTP_Q14 = 0
		}

		for k = 0; k < nStatesDelayedDecision; k++ {
			psDD = &psDelDec[k]

			psSS = &psSampleState[k]

			psDD.Seed = RAND(psDD.Seed)

			dither = RSHIFT(psDD.Seed, 31)

			lpcHistory := psDD.sLPC_Q14[:]
			lpcIndex = int(NSQ_LPC_BUF_LENGTH - 1 + i)

			if predictLPCOrder == 16 {
				LPC_pred_Q10 = nsqLPCPrediction16(lpcHistory, lpcIndex, &aWide)
			} else {
				Atmp = a_Q12_tmp[0]
				LPC_pred_Q10 = SMULWB(lpcHistory[lpcIndex], Atmp)
				LPC_pred_Q10 = SMLAWT(LPC_pred_Q10, lpcHistory[lpcIndex-1], Atmp)
				Atmp = a_Q12_tmp[1]
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, lpcHistory[lpcIndex-2], Atmp)
				LPC_pred_Q10 = SMLAWT(LPC_pred_Q10, lpcHistory[lpcIndex-3], Atmp)
				Atmp = a_Q12_tmp[2]
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, lpcHistory[lpcIndex-4], Atmp)
				LPC_pred_Q10 = SMLAWT(LPC_pred_Q10, lpcHistory[lpcIndex-5], Atmp)
				Atmp = a_Q12_tmp[3]
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, lpcHistory[lpcIndex-6], Atmp)
				LPC_pred_Q10 = SMLAWT(LPC_pred_Q10, lpcHistory[lpcIndex-7], Atmp)
				Atmp = a_Q12_tmp[4]
				LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, lpcHistory[lpcIndex-8], Atmp)
				LPC_pred_Q10 = SMLAWT(LPC_pred_Q10, lpcHistory[lpcIndex-9], Atmp)
				for j = 10; j < predictLPCOrder; j += 2 {
					Atmp = a_Q12_tmp[j>>1]
					LPC_pred_Q10 = SMLAWB(LPC_pred_Q10, lpcHistory[lpcIndex-int(j)], Atmp)
					LPC_pred_Q10 = SMLAWT(LPC_pred_Q10, lpcHistory[lpcIndex-int(j)-1], Atmp)
				}

			}

			if shapingLPCOrder == 16 {
				n_AR_Q10 = nsqShape16(lpcHistory[lpcIndex], &psDD.sAR2_Q14, &shapeWide, signedWarp)
			} else {
				tmp2 = SMLAWB(lpcHistory[lpcIndex], psDD.sAR2_Q14[0], warping_Q16)
				tmp1 = SMLAWB(psDD.sAR2_Q14[0], psDD.sAR2_Q14[1]-tmp2, warping_Q16)
				psDD.sAR2_Q14[0] = tmp2
				n_AR_Q10 = SMULWB(tmp2, int32(shapeCoefficients[0]))
				for j = 2; j < shapingLPCOrder; j += 2 {
					tmp2 = SMLAWB(psDD.sAR2_Q14[j-1], psDD.sAR2_Q14[j+0]-tmp1, warping_Q16)
					psDD.sAR2_Q14[j-1] = tmp1
					n_AR_Q10 = SMLAWB(n_AR_Q10, tmp1, int32(shapeCoefficients[j-1]))
					tmp1 = SMLAWB(psDD.sAR2_Q14[j+0], psDD.sAR2_Q14[j+1]-tmp2, warping_Q16)
					psDD.sAR2_Q14[j+0] = tmp2
					n_AR_Q10 = SMLAWB(n_AR_Q10, tmp2, int32(shapeCoefficients[j]))
				}
				psDD.sAR2_Q14[shapingLPCOrder-1] = tmp1
				n_AR_Q10 = SMLAWB(n_AR_Q10, tmp1, int32(shapeCoefficients[shapingLPCOrder-1]))
			}

			n_AR_Q10 = RSHIFT(n_AR_Q10, 1)
			n_AR_Q10 = SMLAWB(n_AR_Q10, psDD.LF_AR_Q12, Tilt_Q14)

			n_LF_Q10 = LSHIFT(SMULWB(psDD.Shape_Q10[*smpl_buf_idx], LF_shp_Q14), 2)
			n_LF_Q10 = SMLAWT(n_LF_Q10, psDD.LF_AR_Q12, LF_shp_Q14)

			tmp1 = SUB32(LTP_pred_Q14, n_LTP_Q14)
			tmp1 = RSHIFT(tmp1, 4)
			tmp1 = ADD32(tmp1, LPC_pred_Q10)
			tmp1 = SUB32(tmp1, n_AR_Q10)
			tmp1 = SUB32(tmp1, n_LF_Q10)
			r_Q10 = SUB32(input[i], tmp1)

			r_Q10 = (r_Q10 ^ dither) - dither
			r_Q10 = SUB32(r_Q10, offset_Q10)
			r_Q10 = LIMIT_32(r_Q10, -64<<10, 64<<10)

			if r_Q10 < -1536 {
				q1_Q10 = LSHIFT(RSHIFT_ROUND(r_Q10, 10), 10)
				r_Q10 = SUB32(r_Q10, q1_Q10)
				rd1_Q10 = RSHIFT(SMLABB(MUL(-ADD32(q1_Q10, offset_Q10), Lambda_Q10), r_Q10, r_Q10), 10)
				rd2_Q10 = ADD32(rd1_Q10, 1024)
				rd2_Q10 = SUB32(rd2_Q10, ADD_LSHIFT32(Lambda_Q10, r_Q10, 1))
				q2_Q10 = ADD32(q1_Q10, 1024)
			} else if r_Q10 > 512 {
				q1_Q10 = LSHIFT(RSHIFT_ROUND(r_Q10, 10), 10)
				r_Q10 = SUB32(r_Q10, q1_Q10)
				rd1_Q10 = RSHIFT(SMLABB(MUL(ADD32(q1_Q10, offset_Q10), Lambda_Q10), r_Q10, r_Q10), 10)
				rd2_Q10 = ADD32(rd1_Q10, 1024)
				rd2_Q10 = SUB32(rd2_Q10, SUB_LSHIFT32(Lambda_Q10, r_Q10, 1))
				q2_Q10 = SUB32(q1_Q10, 1024)
			} else {
				rr_Q20 = SMULBB(offset_Q10, Lambda_Q10)
				rd2_Q10 = RSHIFT(SMLABB(rr_Q20, r_Q10, r_Q10), 10)
				rd1_Q10 = ADD32(rd2_Q10, 1024)
				rd1_Q10 = ADD32(rd1_Q10, SUB_RSHIFT32(ADD_LSHIFT32(Lambda_Q10, r_Q10, 1), rr_Q20, 9))
				q1_Q10 = -1024
				q2_Q10 = 0
			}

			if rd1_Q10 < rd2_Q10 {
				psSS[0].RD_Q10 = ADD32(psDD.RD_Q10, rd1_Q10)
				psSS[1].RD_Q10 = ADD32(psDD.RD_Q10, rd2_Q10)
				psSS[0].Q_Q10 = q1_Q10
				psSS[1].Q_Q10 = q2_Q10
			} else {
				psSS[0].RD_Q10 = ADD32(psDD.RD_Q10, rd2_Q10)
				psSS[1].RD_Q10 = ADD32(psDD.RD_Q10, rd1_Q10)
				psSS[0].Q_Q10 = q2_Q10
				psSS[1].Q_Q10 = q1_Q10
			}

			exc_Q10 = ADD32(offset_Q10, psSS[0].Q_Q10)
			exc_Q10 = (exc_Q10 ^ dither) - dither

			LPC_exc_Q10 = exc_Q10 + RSHIFT_ROUND(LTP_pred_Q14, 4)
			xq_Q10 = ADD32(LPC_exc_Q10, LPC_pred_Q10)

			sLF_AR_shp_Q10 = SUB32(xq_Q10, n_AR_Q10)
			psSS[0].sLTP_shp_Q10 = SUB32(sLF_AR_shp_Q10, n_LF_Q10)
			psSS[0].LF_AR_Q12 = LSHIFT(sLF_AR_shp_Q10, 2)
			psSS[0].xq_Q14 = LSHIFT(xq_Q10, 4)
			psSS[0].LPC_exc_Q16 = LSHIFT(LPC_exc_Q10, 6)

			exc_Q10 = ADD32(offset_Q10, psSS[1].Q_Q10)
			exc_Q10 = (exc_Q10 ^ dither) - dither

			LPC_exc_Q10 = exc_Q10 + RSHIFT_ROUND(LTP_pred_Q14, 4)
			xq_Q10 = ADD32(LPC_exc_Q10, LPC_pred_Q10)

			sLF_AR_shp_Q10 = SUB32(xq_Q10, n_AR_Q10)
			psSS[1].sLTP_shp_Q10 = SUB32(sLF_AR_shp_Q10, n_LF_Q10)
			psSS[1].LF_AR_Q12 = LSHIFT(sLF_AR_shp_Q10, 2)
			psSS[1].xq_Q14 = LSHIFT(xq_Q10, 4)
			psSS[1].LPC_exc_Q16 = LSHIFT(LPC_exc_Q10, 6)
		}

		*smpl_buf_idx = (*smpl_buf_idx - 1) & DECISION_DELAY_MASK
		last_smple_idx = (*smpl_buf_idx + decisionDelay) & DECISION_DELAY_MASK

		RDmin_Q10 = psSampleState[0][0].RD_Q10
		Winner_ind = 0
		for k = 1; k < nStatesDelayedDecision; k++ {
			if psSampleState[k][0].RD_Q10 < RDmin_Q10 {
				RDmin_Q10 = psSampleState[k][0].RD_Q10
				Winner_ind = k
			}
		}

		Winner_rand_state = psDelDec[Winner_ind].RandState[last_smple_idx]
		for k = 0; k < nStatesDelayedDecision; k++ {
			if psDelDec[k].RandState[last_smple_idx] != Winner_rand_state {
				psSampleState[k][0].RD_Q10 = ADD32(psSampleState[k][0].RD_Q10, math.MaxInt32>>4)
				psSampleState[k][1].RD_Q10 = ADD32(psSampleState[k][1].RD_Q10, math.MaxInt32>>4)
			}
		}

		RDmax_Q10 = psSampleState[0][0].RD_Q10
		RDmin_Q10 = psSampleState[0][1].RD_Q10
		RDmax_ind = 0
		RDmin_ind = 0
		for k = 1; k < nStatesDelayedDecision; k++ {
			if psSampleState[k][0].RD_Q10 > RDmax_Q10 {
				RDmax_Q10 = psSampleState[k][0].RD_Q10
				RDmax_ind = k
			}
			if psSampleState[k][1].RD_Q10 < RDmin_Q10 {
				RDmin_Q10 = psSampleState[k][1].RD_Q10
				RDmin_ind = k
			}
		}

		if RDmin_Q10 < RDmax_Q10 {
			copy_del_dec_state(&psDelDec[RDmax_ind], &psDelDec[RDmin_ind], i)
			psSampleState[RDmin_ind][1].copy(&psSampleState[RDmax_ind][0])
		}

		psDD = &psDelDec[Winner_ind]
		if subfr > 0 || i >= decisionDelay {
			pulseData[pulseOffset+int(i-decisionDelay)] = int8(RSHIFT(psDD.Q_Q10[last_smple_idx], 10))
			outputData[outputOffset+int(i-decisionDelay)] = int16(SAT16(RSHIFT_ROUND(
				SMULWW(psDD.Xq_Q10[last_smple_idx], psDD.Gain_Q16[last_smple_idx]), 10)))
			shapeHistory[NSQ.sLTP_shp_Q10.offset+int(NSQ.sLTP_shp_buf_idx-decisionDelay)] = psDD.Shape_Q10[last_smple_idx]
			predictionHistory[sLTP_Q16.offset+int(NSQ.sLTP_buf_idx-decisionDelay)] = psDD.Pred_Q16[last_smple_idx]
		}
		NSQ.sLTP_shp_buf_idx++
		NSQ.sLTP_buf_idx++

		for k = 0; k < nStatesDelayedDecision; k++ {
			psDD = &psDelDec[k]
			psSS = &psSampleState[k]
			psDD.LF_AR_Q12 = psSS[0].LF_AR_Q12
			psDD.sLPC_Q14[NSQ_LPC_BUF_LENGTH+i] = psSS[0].xq_Q14
			psDD.Xq_Q10[*smpl_buf_idx] = RSHIFT(psSS[0].xq_Q14, 4)
			psDD.Q_Q10[*smpl_buf_idx] = psSS[0].Q_Q10
			psDD.Pred_Q16[*smpl_buf_idx] = psSS[0].LPC_exc_Q16
			psDD.Shape_Q10[*smpl_buf_idx] = psSS[0].sLTP_shp_Q10
			psDD.Seed = ADD_RSHIFT32(psDD.Seed, psSS[0].Q_Q10, 10)
			psDD.RandState[*smpl_buf_idx] = psDD.Seed
			psDD.RD_Q10 = psSS[0].RD_Q10
			psDD.Gain_Q16[*smpl_buf_idx] = Gain_Q16
		}
	}
	for k = 0; k < nStatesDelayedDecision; k++ {
		psDD = &psDelDec[k]
		copy(psDD.sLPC_Q14[:NSQ_LPC_BUF_LENGTH], psDD.sLPC_Q14[length:length+NSQ_LPC_BUF_LENGTH])
	}
}

func nsq_del_dec_scale_states(
	NSQ *nsq_state,
	psDelDec []NSQ_del_dec_struct,
	x *slice[int16],
	x_sc_Q10 *slice[int32],
	subfr_length int32,
	sLTP *slice[int16],
	sLTP_Q16 *slice[int32],
	subfr int32,
	nStatesDelayedDecision int32,
	smpl_buf_idx int32,
	LTP_scale_Q14 int32,
	Gains_Q16 *slice[int32],
	pitchL *slice[int32]) {
	var (
		i, k, lag                                int32
		inv_gain_Q16, gain_adj_Q16, inv_gain_Q32 int32
		psDD                                     *NSQ_del_dec_struct
	)

	prediction := sLTP_Q16.From()
	whitened := sLTP.From()
	shape := NSQ.sLTP_shp_Q10.From()
	input := x.From()[:subfr_length]
	scaled := x_sc_Q10.From()[:subfr_length]

	inv_gain_Q16 = INVERSE32_varQ(max(Gains_Q16.idx(int(subfr)), 1), 32)
	inv_gain_Q16 = min(inv_gain_Q16, math.MaxInt16)
	lag = pitchL.idx(int(subfr))

	if NSQ.rewhite_flag != 0 {
		inv_gain_Q32 = LSHIFT(inv_gain_Q16, 16)
		if subfr == 0 {
			inv_gain_Q32 = LSHIFT(SMULWB(inv_gain_Q32, LTP_scale_Q14), 2)
		}
		for i = NSQ.sLTP_buf_idx - lag - LTP_ORDER/2; i < NSQ.sLTP_buf_idx; i++ {
			prediction[i] = SMULWB(inv_gain_Q32, int32(whitened[i]))
		}
	}

	if inv_gain_Q16 != NSQ.prev_inv_gain_Q16 {
		gain_adj_Q16 = DIV32_varQ(inv_gain_Q16, NSQ.prev_inv_gain_Q16, 16)

		for i = NSQ.sLTP_shp_buf_idx - subfr_length*NB_SUBFR; i < NSQ.sLTP_shp_buf_idx; i++ {
			shape[i] = SMULWW(gain_adj_Q16, shape[i])
		}

		if NSQ.rewhite_flag == 0 {
			for i = NSQ.sLTP_buf_idx - lag - LTP_ORDER/2; i < NSQ.sLTP_buf_idx; i++ {
				prediction[i] = SMULWW(gain_adj_Q16, prediction[i])
			}
		}

		for k = 0; k < nStatesDelayedDecision; k++ {
			psDD = &psDelDec[k]

			psDD.LF_AR_Q12 = SMULWW(gain_adj_Q16, psDD.LF_AR_Q12)

			for i = 0; i < NSQ_LPC_BUF_LENGTH; i++ {
				psDD.sLPC_Q14[i] = SMULWW(gain_adj_Q16, psDD.sLPC_Q14[i])
			}
			for i = 0; i < MAX_SHAPE_LPC_ORDER; i++ {
				psDD.sAR2_Q14[i] = SMULWW(gain_adj_Q16, psDD.sAR2_Q14[i])
			}
			for i = 0; i < DECISION_DELAY; i++ {
				psDD.Pred_Q16[i] = SMULWW(gain_adj_Q16, psDD.Pred_Q16[i])
				psDD.Shape_Q10[i] = SMULWW(gain_adj_Q16, psDD.Shape_Q10[i])
			}
		}
	}

	for i = 0; i < subfr_length; i++ {
		scaled[i] = RSHIFT(SMULBB(int32(input[i]), inv_gain_Q16), 6)
	}

	NSQ.prev_inv_gain_Q16 = inv_gain_Q16
}

func copy_del_dec_state(DD_dst *NSQ_del_dec_struct, DD_src *NSQ_del_dec_struct, LPC_state_idx int32) {
	DD_dst.RandState = DD_src.RandState
	DD_dst.Q_Q10 = DD_src.Q_Q10
	DD_dst.Pred_Q16 = DD_src.Pred_Q16
	DD_dst.Shape_Q10 = DD_src.Shape_Q10
	DD_dst.Xq_Q10 = DD_src.Xq_Q10
	DD_dst.sAR2_Q14 = DD_src.sAR2_Q14
	copy(DD_dst.sLPC_Q14[LPC_state_idx:LPC_state_idx+NSQ_LPC_BUF_LENGTH],
		DD_src.sLPC_Q14[LPC_state_idx:LPC_state_idx+NSQ_LPC_BUF_LENGTH])

	DD_dst.LF_AR_Q12 = DD_src.LF_AR_Q12
	DD_dst.Seed = DD_src.Seed
	DD_dst.SeedInit = DD_src.SeedInit
	DD_dst.RD_Q10 = DD_src.RD_Q10
}

// Every product is shifted independently before int32 accumulation. Grouping
// only the additions preserves modulo-2^32 wraparound and fixed-point rounding.
func nsqLPCPrediction16(history []int32, index int, coefficients *[MAX_LPC_ORDER]int64) int32 {
	h := history[index-15 : index+1]
	a := int32((int64(h[15])*coefficients[0])>>16) + int32((int64(h[11])*coefficients[4])>>16) + int32((int64(h[7])*coefficients[8])>>16) + int32((int64(h[3])*coefficients[12])>>16)
	b := int32((int64(h[14])*coefficients[1])>>16) + int32((int64(h[10])*coefficients[5])>>16) + int32((int64(h[6])*coefficients[9])>>16) + int32((int64(h[2])*coefficients[13])>>16)
	c := int32((int64(h[13])*coefficients[2])>>16) + int32((int64(h[9])*coefficients[6])>>16) + int32((int64(h[5])*coefficients[10])>>16) + int32((int64(h[1])*coefficients[14])>>16)
	d := int32((int64(h[12])*coefficients[3])>>16) + int32((int64(h[8])*coefficients[7])>>16) + int32((int64(h[4])*coefficients[11])>>16) + int32((int64(h[0])*coefficients[15])>>16)
	return (a + b) + (c + d)
}

// The 16-stage all-pass chain is sequential; expand its fixed bounds while
// retaining every state update, subtraction wraparound and product shift.
func nsqShape16(input int32, state *[MAX_SHAPE_LPC_ORDER]int32, coefficients *[MAX_SHAPE_LPC_ORDER]int64, warp int64) int32 {
	tmp2 := input + int32((int64(state[0])*warp)>>16)
	tmp1 := state[0] + int32((int64(state[1]-tmp2)*warp)>>16)
	state[0] = tmp2
	sum := int32((int64(tmp2) * coefficients[0]) >> 16)
	tmp2 = state[1] + int32((int64(state[2]-tmp1)*warp)>>16)
	state[1] = tmp1
	sum += int32((int64(tmp1) * coefficients[1]) >> 16)
	tmp1 = state[2] + int32((int64(state[3]-tmp2)*warp)>>16)
	state[2] = tmp2
	sum += int32((int64(tmp2) * coefficients[2]) >> 16)
	tmp2 = state[3] + int32((int64(state[4]-tmp1)*warp)>>16)
	state[3] = tmp1
	sum += int32((int64(tmp1) * coefficients[3]) >> 16)
	tmp1 = state[4] + int32((int64(state[5]-tmp2)*warp)>>16)
	state[4] = tmp2
	sum += int32((int64(tmp2) * coefficients[4]) >> 16)
	tmp2 = state[5] + int32((int64(state[6]-tmp1)*warp)>>16)
	state[5] = tmp1
	sum += int32((int64(tmp1) * coefficients[5]) >> 16)
	tmp1 = state[6] + int32((int64(state[7]-tmp2)*warp)>>16)
	state[6] = tmp2
	sum += int32((int64(tmp2) * coefficients[6]) >> 16)
	tmp2 = state[7] + int32((int64(state[8]-tmp1)*warp)>>16)
	state[7] = tmp1
	sum += int32((int64(tmp1) * coefficients[7]) >> 16)
	tmp1 = state[8] + int32((int64(state[9]-tmp2)*warp)>>16)
	state[8] = tmp2
	sum += int32((int64(tmp2) * coefficients[8]) >> 16)
	tmp2 = state[9] + int32((int64(state[10]-tmp1)*warp)>>16)
	state[9] = tmp1
	sum += int32((int64(tmp1) * coefficients[9]) >> 16)
	tmp1 = state[10] + int32((int64(state[11]-tmp2)*warp)>>16)
	state[10] = tmp2
	sum += int32((int64(tmp2) * coefficients[10]) >> 16)
	tmp2 = state[11] + int32((int64(state[12]-tmp1)*warp)>>16)
	state[11] = tmp1
	sum += int32((int64(tmp1) * coefficients[11]) >> 16)
	tmp1 = state[12] + int32((int64(state[13]-tmp2)*warp)>>16)
	state[12] = tmp2
	sum += int32((int64(tmp2) * coefficients[12]) >> 16)
	tmp2 = state[13] + int32((int64(state[14]-tmp1)*warp)>>16)
	state[13] = tmp1
	sum += int32((int64(tmp1) * coefficients[13]) >> 16)
	tmp1 = state[14] + int32((int64(state[15]-tmp2)*warp)>>16)
	state[14] = tmp2
	sum += int32((int64(tmp2) * coefficients[14]) >> 16)
	state[MAX_SHAPE_LPC_ORDER-1] = tmp1
	return sum + int32((int64(tmp1)*coefficients[MAX_SHAPE_LPC_ORDER-1])>>16)
}
