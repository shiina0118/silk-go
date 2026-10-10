package silk

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestPredictionFiltersMatchOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(923441))
	for _, order := range []int32{2, 4, 8, 10, 12, 16} {
		for _, length := range []int32{0, 1, 2, 13, 80, 120, 160, 240, 360, 544} {
			for _, warp := range []int16{-32768, -16384, 0, 6554, 9830, 16384, 32767} {
				input := make([]int16, length+8)
				for i := range input {
					input[i] = int16(rng.Int31())
				}
				want, got := make([]int32, order+6), make([]int32, order+6)
				for i := range want {
					want[i] = -192837
					got[i] = -192837
				}
				in := &slice[int16]{arr: input, offset: 3, size: 2}
				var wantScale, gotScale int32
				referenceWarpedAutocorrelation(&slice[int32]{arr: want, offset: 2, size: 4}, &wantScale, in, warp, length, order)
				warped_autocorrelation_FIX(&slice[int32]{arr: got, offset: 2, size: 4}, &gotScale, in, warp, length, order)
				if gotScale != wantScale || !reflect.DeepEqual(got, want) {
					t.Fatalf("warped order=%d length=%d warp=%d: scale %d != %d or samples differ", order, length, warp, gotScale, wantScale)
				}
			}
			for _, alias := range []bool{false, true} {
				for trial := 0; trial < 8; trial++ {
					input := make([]int16, length+8)
					coeff := make([]int16, order+4)
					wantState := make([]int16, order+4)
					for i := range input {
						input[i] = int16(rng.Int31())
					}
					for i := range coeff {
						coeff[i] = int16(rng.Int31())
					}
					for i := range wantState {
						wantState[i] = int16(rng.Int31())
					}
					gotState := append([]int16(nil), wantState...)
					wantInput := append([]int16(nil), input...)
					gotInput := append([]int16(nil), input...)
					want, got := make([]int16, length+8), make([]int16, length+8)
					if alias {
						want = wantInput
						got = gotInput
					}
					referenceLPCAnalysisFilter(&slice[int16]{arr: wantInput, offset: 3, size: 2}, &slice[int16]{arr: coeff, offset: 2, size: 2}, &slice[int16]{arr: wantState, offset: 2, size: 2}, &slice[int16]{arr: want, offset: 3, size: 2}, length, order)
					LPC_analysis_filter(&slice[int16]{arr: gotInput, offset: 3, size: 2}, &slice[int16]{arr: coeff, offset: 2, size: 2}, &slice[int16]{arr: gotState, offset: 2, size: 2}, &slice[int16]{arr: got, offset: 3, size: 2}, length, order)
					if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotState, wantState) {
						t.Fatalf("LPC order=%d length=%d alias=%v trial=%d", order, length, alias, trial)
					}
				}
			}
		}
	}
	for _, order := range []int32{1, 2, 4, 8, 10, 12, 16} {
		for _, length := range []int32{0, 1, 2, 13, 80, 120, 160, 240, 360, 544} {
			input := make([]int16, length+8)
			coeff := make([]int16, order+4)
			wantState := make([]int32, order+4)
			for i := range input {
				input[i] = int16(rng.Int31())
			}
			for i := range coeff {
				coeff[i] = int16(rng.Int31())
			}
			for i := range wantState {
				wantState[i] = int32(rng.Uint32())
			}
			gotState := append([]int32(nil), wantState...)
			want, got := make([]int16, length+8), make([]int16, length+8)
			in := &slice[int16]{arr: input, offset: 3, size: 2}
			b := &slice[int16]{arr: coeff, offset: 2, size: 2}
			referenceMAPrediction(in, b, &slice[int32]{arr: wantState, offset: 2, size: 4}, &slice[int16]{arr: want, offset: 3, size: 2}, length, order)
			MA_Prediction(in, b, &slice[int32]{arr: gotState, offset: 2, size: 4}, &slice[int16]{arr: got, offset: 3, size: 2}, length, order)
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotState, wantState) {
				t.Fatalf("MA order=%d length=%d", order, length)
			}
		}
	}
}

func referenceWarpedAutocorrelation(corr *slice[int32], scale *int32, input *slice[int16], warping_Q16 int16, length, order int32) {
	var (
		n, i, lsh        int32
		tmp1_QS, tmp2_QS int32
		state_QS         = alloc[int32](MAX_SHAPE_LPC_ORDER + 1)
		corr_QC          = alloc[int64](MAX_SHAPE_LPC_ORDER + 1)
	)

	for n = 0; n < length; n++ {
		tmp1_QS = LSHIFT32(int32(input.idx(int(n))), QS)
		for i = 0; i < order; i += 2 {
			tmp2_QS = SMLAWB(state_QS.idx(int(i)), state_QS.idx(int(i+1))-tmp1_QS, int32(warping_Q16))
			*state_QS.ptr(int(i)) = tmp1_QS
			*corr_QC.ptr(int(i)) += RSHIFT64(SMULL(tmp1_QS, state_QS.idx(0)), 2*QS-QC)
			tmp1_QS = SMLAWB(state_QS.idx(int(i+1)), state_QS.idx(int(i+2))-tmp2_QS, int32(warping_Q16))
			*state_QS.ptr(int(i + 1)) = tmp2_QS
			*corr_QC.ptr(int(i + 1)) += RSHIFT64(SMULL(tmp2_QS, state_QS.idx(0)), 2*QS-QC)
		}
		*state_QS.ptr(int(order)) = tmp1_QS
		*corr_QC.ptr(int(order)) += RSHIFT64(SMULL(tmp1_QS, state_QS.idx(0)), 2*QS-QC)
	}

	lsh = CLZ64(corr_QC.idx(0)) - 35
	lsh = LIMIT(lsh, -12-QC, 30-QC)
	*scale = -(QC + lsh)

	if lsh >= 0 {
		for i = 0; i < order+1; i++ {
			*corr.ptr(int(i)) = int32(LSHIFT64(corr_QC.idx(int(i)), lsh))
		}
	} else {
		for i = 0; i < order+1; i++ {
			*corr.ptr(int(i)) = int32(RSHIFT64(corr_QC.idx(int(i)), -lsh))
		}
	}
}

func referenceMAPrediction(in, B *slice[int16], S *slice[int32], out *slice[int16], length, order int32) {
	var (
		k, d, in16 int32
		out32      int32
	)
	for k = 0; k < length; k++ {
		in16 = int32(in.idx(int(k)))
		out32 = LSHIFT(in16, 12) - S.idx(0)
		out32 = RSHIFT_ROUND(out32, 12)

		for d = 0; d < order-1; d++ {
			*S.ptr(int(d)) = SMLABB_ovflw(S.idx(int(d+1)), in16, int32(B.idx(int(d))))
		}
		*S.ptr(int(order - 1)) = SMULBB(in16, int32(B.idx(int(order-1))))

		*out.ptr(int(k)) = int16(SAT16(out32))
	}
}

func referenceLPCAnalysisFilter(
	in, B, S, out *slice[int16],
	length, Order int32) {
	var (
		k, j, idx        int32
		Order_half       = RSHIFT(Order, 1)
		out32_Q12, out32 int32
		SA, SB           int16
	)

	for k = 0; k < length; k++ {
		SA = S.idx(0)
		out32_Q12 = 0
		for j = 0; j < (Order_half - 1); j++ {
			idx = SMULBB(2, j) + 1
			SB = S.idx(int(idx))
			*S.ptr(int(idx)) = SA
			out32_Q12 = SMLABB(out32_Q12, int32(SA), int32(B.idx(int(idx-1))))
			out32_Q12 = SMLABB(out32_Q12, int32(SB), int32(B.idx(int(idx))))
			SA = S.idx(int(idx + 1))
			*S.ptr(int(idx + 1)) = SB
		}

		SB = S.idx(int(Order - 1))
		*S.ptr(int(Order - 1)) = SA
		out32_Q12 = SMLABB(out32_Q12, int32(SA), int32(B.idx(int(Order-2))))
		out32_Q12 = SMLABB(out32_Q12, int32(SB), int32(B.idx(int(Order-1))))

		out32_Q12 = SUB_SAT32(LSHIFT(int32(in.idx(int(k))), 12), out32_Q12)

		out32 = RSHIFT_ROUND(out32_Q12, 12)

		*out.ptr(int(k)) = int16(SAT16(out32))

		*S.ptr(0) = in.idx(int(k))
	}
}

func referenceBurgModified(res_nrg, res_nrg_Q *int32,
	A_Q16 *slice[int32], x *slice[int16],
	subfr_length, nb_subfr, WhiteNoiseFrac_Q32, D int32) {
	var (
		k, n, s, lz, rshifts, rshifts_extra                      int32
		C0, num, nrg, rc_Q31, Atmp_QA, Atmp1, tmp1, tmp2, x1, x2 int32
		x_ptr                                                    *slice[int16]
		C_first_row                                              = alloc[int32](MAX_ORDER_LPC)
		C_last_row                                               = alloc[int32](MAX_ORDER_LPC)
		Af_QA                                                    = alloc[int32](MAX_ORDER_LPC)
		CAf                                                      = alloc[int32](MAX_ORDER_LPC + 1)
		CAb                                                      = alloc[int32](MAX_ORDER_LPC + 1)
	)

	sum_sqr_shift(&C0, &rshifts, x, nb_subfr*subfr_length)
	if rshifts > MAX_RSHIFTS {
		C0 = LSHIFT32(C0, rshifts-MAX_RSHIFTS)
		rshifts = MAX_RSHIFTS
	} else {
		lz = CLZ32(C0) - 1
		rshifts_extra = N_BITS_HEAD_ROOM - lz
		if rshifts_extra > 0 {
			rshifts_extra = min(rshifts_extra, MAX_RSHIFTS-rshifts)
			C0 = RSHIFT32(C0, rshifts_extra)
		} else {
			rshifts_extra = max(rshifts_extra, MIN_RSHIFTS-rshifts)
			C0 = LSHIFT32(C0, -rshifts_extra)
		}
		rshifts += rshifts_extra
	}

	if rshifts > 0 {
		for s = 0; s < nb_subfr; s++ {
			x_ptr = x.off(int(s * subfr_length))
			for n = 1; n < D+1; n++ {
				*C_first_row.ptr(int(n - 1)) += int32(RSHIFT64(
					inner_prod16_aligned_64(x_ptr, x_ptr.off(int(n)), subfr_length-n), rshifts))
			}
		}
	} else {
		for s = 0; s < nb_subfr; s++ {
			x_ptr = x.off(int(s * subfr_length))
			for n = 1; n < D+1; n++ {
				*C_first_row.ptr(int(n - 1)) += LSHIFT32(
					inner_prod_aligned(x_ptr, x_ptr.off(int(n)), subfr_length-n), -rshifts)
			}
		}
	}
	C_first_row.copy(C_last_row, MAX_ORDER_LPC)

	*CAb.ptr(0) = C0 + SMMUL(WhiteNoiseFrac_Q32, C0) + 1
	*CAf.ptr(0) = CAb.idx(0)

	for n = 0; n < D; n++ {
		if rshifts > -2 {
			for s = 0; s < nb_subfr; s++ {
				x_ptr = x.off(int(s * subfr_length))
				x1 = -LSHIFT32(int32(x_ptr.idx(int(n))), 16-rshifts)
				x2 = -LSHIFT32(int32(x_ptr.idx(int(subfr_length-n-1))), 16-rshifts)
				tmp1 = LSHIFT32(int32(x_ptr.idx(int(n))), BURG_MODIFIED_QA-16)
				tmp2 = LSHIFT32(int32(x_ptr.idx(int(subfr_length-n-1))), BURG_MODIFIED_QA-16)
				for k = 0; k < n; k++ {
					*C_first_row.ptr(int(k)) = SMLAWB(C_first_row.idx(int(k)), x1, int32(x_ptr.idx(int(n-k-1))))
					*C_last_row.ptr(int(k)) = SMLAWB(C_last_row.idx(int(k)), x2, int32(x_ptr.idx(int(subfr_length-n+k))))
					Atmp_QA = Af_QA.idx(int(k))
					tmp1 = SMLAWB(tmp1, Atmp_QA, int32(x_ptr.idx(int(n-k-1))))
					tmp2 = SMLAWB(tmp2, Atmp_QA, int32(x_ptr.idx(int(subfr_length-n+k))))
				}
				tmp1 = LSHIFT32(-tmp1, 32-BURG_MODIFIED_QA-rshifts)
				tmp2 = LSHIFT32(-tmp2, 32-BURG_MODIFIED_QA-rshifts)
				for k = 0; k <= n; k++ {
					*CAf.ptr(int(k)) = SMLAWB(CAf.idx(int(k)), tmp1, int32(x_ptr.idx(int(n-k))))
					*CAb.ptr(int(k)) = SMLAWB(CAb.idx(int(k)), tmp2, int32(x_ptr.idx(int(subfr_length-n+k-1))))
				}
			}
		} else {
			for s = 0; s < nb_subfr; s++ {
				x_ptr = x.off(int(s * subfr_length))
				x1 = -LSHIFT32(int32(x_ptr.idx(int(n))), -rshifts)
				x2 = -LSHIFT32(int32(x_ptr.idx(int(subfr_length-n-1))), -rshifts)
				tmp1 = LSHIFT32(int32(x_ptr.idx(int(n))), 17)
				tmp2 = LSHIFT32(int32(x_ptr.idx(int(subfr_length-n-1))), 17)
				for k = 0; k < n; k++ {
					*C_first_row.ptr(int(k)) = MLA(C_first_row.idx(int(k)), x1, int32(x_ptr.idx(int(n-k-1))))
					*C_last_row.ptr(int(k)) = MLA(C_last_row.idx(int(k)), x2, int32(x_ptr.idx(int(subfr_length-n+k))))
					Atmp1 = RSHIFT_ROUND(Af_QA.idx(int(k)), BURG_MODIFIED_QA-17)
					tmp1 = MLA(tmp1, int32(x_ptr.idx(int(n-k-1))), Atmp1)
					tmp2 = MLA(tmp2, int32(x_ptr.idx(int(subfr_length-n+k))), Atmp1)
				}
				tmp1 = -tmp1
				tmp2 = -tmp2
				for k = 0; k <= n; k++ {
					*CAf.ptr(int(k)) = SMLAWW(CAf.idx(int(k)), tmp1,
						LSHIFT32(int32(x_ptr.idx(int(n-k))), -rshifts-1))
					*CAb.ptr(int(k)) = SMLAWW(CAb.idx(int(k)), tmp2,
						LSHIFT32(int32(x_ptr.idx(int(subfr_length-n+k-1))), -rshifts-1))
				}
			}
		}

		tmp1 = C_first_row.idx(int(n))
		tmp2 = C_last_row.idx(int(n))
		num = 0
		nrg = ADD32(CAb.idx(0), CAf.idx(0))
		for k = 0; k < n; k++ {
			Atmp_QA = Af_QA.idx(int(k))
			lz = CLZ32(abs(Atmp_QA)) - 1
			lz = min(32-BURG_MODIFIED_QA, lz)
			Atmp1 = LSHIFT32(Atmp_QA, lz)

			tmp1 = ADD_LSHIFT32(tmp1, SMMUL(C_last_row.idx(int(n-k-1)), Atmp1), 32-BURG_MODIFIED_QA-lz)
			tmp2 = ADD_LSHIFT32(tmp2, SMMUL(C_first_row.idx(int(n-k-1)), Atmp1), 32-BURG_MODIFIED_QA-lz)
			num = ADD_LSHIFT32(num, SMMUL(CAb.idx(int(n-k)), Atmp1), 32-BURG_MODIFIED_QA-lz)
			nrg = ADD_LSHIFT32(nrg, SMMUL(ADD32(CAb.idx(int(k+1)), CAf.idx(int(k+1))),
				Atmp1), 32-BURG_MODIFIED_QA-lz)
		}
		*CAf.ptr(int(n + 1)) = tmp1
		*CAb.ptr(int(n + 1)) = tmp2
		num = ADD32(num, tmp2)
		num = LSHIFT32(-num, 1)

		if abs(num) < nrg {
			rc_Q31 = DIV32_varQ(num, nrg, 31)
		} else {
			memset(Af_QA.off(int(n)), 0, int(D-n))
			break
		}

		for k = 0; k < (n+1)>>1; k++ {
			tmp1 = Af_QA.idx(int(k))
			tmp2 = Af_QA.idx(int(n - k - 1))
			*Af_QA.ptr(int(k)) = ADD_LSHIFT32(tmp1, SMMUL(tmp2, rc_Q31), 1)
			*Af_QA.ptr(int(n - k - 1)) = ADD_LSHIFT32(tmp2, SMMUL(tmp1, rc_Q31), 1)
		}
		*Af_QA.ptr(int(n)) = RSHIFT32(rc_Q31, 31-BURG_MODIFIED_QA)

		for k = 0; k <= n+1; k++ {
			tmp1 = CAf.idx(int(k))
			tmp2 = CAb.idx(int(n - k + 1))
			*CAf.ptr(int(k)) = ADD_LSHIFT32(tmp1, SMMUL(tmp2, rc_Q31), 1)
			*CAb.ptr(int(n - k + 1)) = ADD_LSHIFT32(tmp2, SMMUL(tmp1, rc_Q31), 1)
		}
	}

	nrg = CAf.idx(0)
	tmp1 = 1 << 16
	for k = 0; k < D; k++ {
		Atmp1 = RSHIFT_ROUND(Af_QA.idx(int(k)), BURG_MODIFIED_QA-16)
		nrg = SMLAWW(nrg, CAf.idx(int(k+1)), Atmp1)
		tmp1 = SMLAWW(tmp1, Atmp1, Atmp1)
		*A_Q16.ptr(int(k)) = -Atmp1
	}
	*res_nrg = SMLAWW(nrg, SMMUL(WhiteNoiseFrac_Q32, C0), -tmp1)
	*res_nrg_Q = -rshifts
}

func referenceFindLPC(
	NLSF_Q15 *slice[int32],
	interpIndex *int32,
	prev_NLSFq_Q15 *slice[int32],
	useInterpolatedNLSFs, LPC_order int32,
	x *slice[int16],
	subfr_length int32) {
	var (
		k                                          int32
		a_Q16                                      = alloc[int32](MAX_LPC_ORDER)
		isInterpLower, shift                       int32
		S                                          = alloc[int16](MAX_LPC_ORDER)
		res_nrg0, res_nrg1                         int32
		rshift0, rshift1                           int32
		a_tmp_Q16                                  = alloc[int32](MAX_LPC_ORDER)
		res_nrg_interp, res_nrg, res_tmp_nrg       int32
		res_nrg_interp_Q, res_nrg_Q, res_tmp_nrg_Q int32
		a_tmp_Q12                                  = alloc[int16](MAX_LPC_ORDER)
		NLSF0_Q15                                  = alloc[int32](MAX_LPC_ORDER)
		LPC_res                                    = alloc[int16]((MAX_FRAME_LENGTH + NB_SUBFR*MAX_LPC_ORDER) / 2)
	)

	*interpIndex = 4

	referenceBurgModified(&res_nrg, &res_nrg_Q, a_Q16, x, subfr_length, NB_SUBFR, FIX_CONST(FIND_LPC_COND_FAC, 32), LPC_order)

	bwexpander_32(a_Q16, LPC_order, FIX_CONST(FIND_LPC_CHIRP, 16))

	if useInterpolatedNLSFs == 1 {

		referenceBurgModified(&res_tmp_nrg, &res_tmp_nrg_Q, a_tmp_Q16, x.off(int((NB_SUBFR>>1)*subfr_length)),
			subfr_length, (NB_SUBFR >> 1), FIX_CONST(FIND_LPC_COND_FAC, 32), LPC_order)

		bwexpander_32(a_tmp_Q16, LPC_order, FIX_CONST(FIND_LPC_CHIRP, 16))

		shift = res_tmp_nrg_Q - res_nrg_Q
		if shift >= 0 {
			if shift < 32 {
				res_nrg = res_nrg - RSHIFT(res_tmp_nrg, shift)
			}
		} else {
			res_nrg = RSHIFT(res_nrg, -shift) - res_tmp_nrg
			res_nrg_Q = res_tmp_nrg_Q
		}

		A2NLSF(NLSF_Q15, a_tmp_Q16, LPC_order)

		for k = 3; k >= 0; k-- {
			interpolate(NLSF0_Q15, prev_NLSFq_Q15, NLSF_Q15, k, LPC_order)

			NLSF2A_stable(a_tmp_Q12, NLSF0_Q15, LPC_order)

			memset(S, 0, int(LPC_order))
			referenceLPCAnalysisFilter(x, a_tmp_Q12, S, LPC_res, 2*subfr_length, LPC_order)

			sum_sqr_shift(&res_nrg0, &rshift0, LPC_res.off(int(LPC_order)), subfr_length-LPC_order)
			sum_sqr_shift(&res_nrg1, &rshift1, LPC_res.off(int(LPC_order+subfr_length)), subfr_length-LPC_order)

			shift = rshift0 - rshift1
			if shift >= 0 {
				res_nrg1 = RSHIFT(res_nrg1, shift)
				res_nrg_interp_Q = -rshift0
			} else {
				res_nrg0 = RSHIFT(res_nrg0, -shift)
				res_nrg_interp_Q = -rshift1
			}
			res_nrg_interp = ADD32(res_nrg0, res_nrg1)

			shift = res_nrg_interp_Q - res_nrg_Q
			if shift >= 0 {
				if RSHIFT(res_nrg_interp, shift) < res_nrg {
					isInterpLower = 1
				} else {
					isInterpLower = 0
				}
			} else {
				if -shift < 32 {
					if res_nrg_interp < RSHIFT(res_nrg, -shift) {
						isInterpLower = 1
					} else {
						isInterpLower = 0
					}
				} else {
					isInterpLower = 0
				}
			}

			if isInterpLower == 1 {
				res_nrg = res_nrg_interp
				res_nrg_Q = res_nrg_interp_Q
				*interpIndex = k
			}
		}
	}

	if *interpIndex == 4 {
		A2NLSF(NLSF_Q15, a_Q16, LPC_order)
	}
}

func referenceNLSFDecode(pNLSF_Q15 *slice[int32], psNLSF_CB *NLSF_CB_struct, NLSFIndices *slice[int32], LPC_order int32) {
	var (
		pCB_element *slice[int16]
		s, i        int32
	)

	pCB_element = psNLSF_CB.CBStages[0].CB_NLSF_Q15.off(int(MUL(NLSFIndices.idx(0), LPC_order)))

	for i = 0; i < LPC_order; i++ {
		*pNLSF_Q15.ptr(int(i)) = int32(pCB_element.idx(int(i)))
	}

	for s = 1; s < psNLSF_CB.nStages; s++ {

		if LPC_order == 16 {
			pCB_element = psNLSF_CB.CBStages[s].CB_NLSF_Q15.off(int(LSHIFT(NLSFIndices.idx(int(s)), 4)))

			*pNLSF_Q15.ptr(0) += int32(pCB_element.idx(0))
			*pNLSF_Q15.ptr(1) += int32(pCB_element.idx(1))
			*pNLSF_Q15.ptr(2) += int32(pCB_element.idx(2))
			*pNLSF_Q15.ptr(3) += int32(pCB_element.idx(3))
			*pNLSF_Q15.ptr(4) += int32(pCB_element.idx(4))
			*pNLSF_Q15.ptr(5) += int32(pCB_element.idx(5))
			*pNLSF_Q15.ptr(6) += int32(pCB_element.idx(6))
			*pNLSF_Q15.ptr(7) += int32(pCB_element.idx(7))
			*pNLSF_Q15.ptr(8) += int32(pCB_element.idx(8))
			*pNLSF_Q15.ptr(9) += int32(pCB_element.idx(9))
			*pNLSF_Q15.ptr(10) += int32(pCB_element.idx(10))
			*pNLSF_Q15.ptr(11) += int32(pCB_element.idx(11))
			*pNLSF_Q15.ptr(12) += int32(pCB_element.idx(12))
			*pNLSF_Q15.ptr(13) += int32(pCB_element.idx(13))
			*pNLSF_Q15.ptr(14) += int32(pCB_element.idx(14))
			*pNLSF_Q15.ptr(15) += int32(pCB_element.idx(15))
		} else {
			pCB_element = psNLSF_CB.CBStages[s].CB_NLSF_Q15.off(int(SMULBB(NLSFIndices.idx(int(s)), LPC_order)))

			for i = 0; i < LPC_order; i++ {
				*pNLSF_Q15.ptr(int(i)) += int32(pCB_element.idx(int(i)))
			}
		}
	}

	NLSF_stabilize(pNLSF_Q15, psNLSF_CB.NDeltaMin_Q15, LPC_order)
}

func referenceNLSFEncode(NLSFIndices, pNLSF_Q15 *slice[int32], psNLSF_CB *NLSF_CB_struct,
	pNLSF_q_Q15_prev, pW_Q6 *slice[int32], NLSF_mu_Q15, NLSF_mu_fluc_red_Q16, NLSF_MSVQ_Survivors, LPC_order, deactivate_fluc_red int32) {
	var (
		i, s, k, cur_survivors, prev_survivors, min_survivors, input_index, cb_index, bestIndex int32
		rateDistThreshold_Q18                                                                   int32
		se_Q15, wsse_Q20, bestRateDist_Q20                                                      int32
		pRateDist_Q18                                                                           = alloc[int32](NLSF_MSVQ_TREE_SEARCH_MAX_VECTORS_EVALUATED)
		pRate_Q5                                                                                = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS)
		pRate_new_Q5                                                                            = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS)
		pTempIndices                                                                            = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS)
		pPath                                                                                   = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS * NLSF_MSVQ_MAX_CB_STAGES)
		pPath_new                                                                               = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS * NLSF_MSVQ_MAX_CB_STAGES)
		pRes_Q15                                                                                = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS * MAX_LPC_ORDER)
		pRes_new_Q15                                                                            = alloc[int32](MAX_NLSF_MSVQ_SURVIVORS * MAX_LPC_ORDER)
		pCurrentCBStage                                                                         *NLSF_CBS
	)
	residuals := pRes_Q15.From()
	newResiduals := pRes_new_Q15.From()
	paths := pPath.From()
	newPaths := pPath_new.From()
	rates := pRate_Q5.From()
	newRates := pRate_new_Q5.From()
	temporaryIndices := pTempIndices.From()

	memset(pRate_Q5, 0, int(NLSF_MSVQ_Survivors))

	for i = 0; i < LPC_order; i++ {
		residuals[i] = pNLSF_Q15.idx(int(i))
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

		if pRateDist_Q18.idx(0) < math.MaxInt32/MAX_NLSF_MSVQ_SURVIVORS {
			rateDistThreshold_Q18 = SMLAWB(pRateDist_Q18.idx(0),
				MUL(NLSF_MSVQ_Survivors, pRateDist_Q18.idx(0)), FIX_CONST(NLSF_MSVQ_SURV_MAX_REL_RD, 16))
			for pRateDist_Q18.idx(int(cur_survivors-1)) > rateDistThreshold_Q18 && cur_survivors > min_survivors {
				cur_survivors--
			}
		}
		for k = 0; k < cur_survivors; k++ {
			if s > 0 {
				if pCurrentCBStage.nVectors == 8 {
					input_index = RSHIFT(pTempIndices.idx(int(k)), 3)
					cb_index = pTempIndices.idx(int(k)) & 7
				} else {
					input_index = DIV32_16(pTempIndices.idx(int(k)), int16(pCurrentCBStage.nVectors))
					cb_index = pTempIndices.idx(int(k)) - SMULBB(input_index, pCurrentCBStage.nVectors)
				}
			} else {
				input_index = 0
				cb_index = temporaryIndices[k]
			}

			residualOffset := input_index * LPC_order
			codebookOffset := cb_index * LPC_order
			newResidualOffset := k * LPC_order
			codebook := pCurrentCBStage.CB_NLSF_Q15.From()
			for i = 0; i < LPC_order; i++ {
				newResiduals[newResidualOffset+i] = residuals[residualOffset+i] - int32(codebook[codebookOffset+i])
			}

			newRates[k] = rates[input_index] + int32(pCurrentCBStage.Rates_Q5.idx(int(cb_index)))

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
			referenceNLSFDecode(pNLSF_Q15, psNLSF_CB, pPath_new.off(int(SMULBB(s, psNLSF_CB.nStages))), LPC_order)

			wsse_Q20 = 0
			for i = 0; i < LPC_order; i += 2 {
				se_Q15 = pNLSF_Q15.idx(int(i)) - pNLSF_q_Q15_prev.idx(int(i))
				wsse_Q20 = SMLAWB(wsse_Q20, SMULBB(se_Q15, se_Q15), pW_Q6.idx(int(i)))

				se_Q15 = pNLSF_Q15.idx(int(i+1)) - pNLSF_q_Q15_prev.idx(int(i+1))
				wsse_Q20 = SMLAWB(wsse_Q20, SMULBB(se_Q15, se_Q15), pW_Q6.idx(int(i+1)))
			}

			wsse_Q20 = ADD_POS_SAT32(pRateDist_Q18.idx(int(s)), SMULWB(wsse_Q20, NLSF_mu_fluc_red_Q16))

			if wsse_Q20 < bestRateDist_Q20 {
				bestRateDist_Q20 = wsse_Q20
				bestIndex = s
			}
		}
	}

	bestPathOffset := int(bestIndex * psNLSF_CB.nStages)
	copy(NLSFIndices.From()[:psNLSF_CB.nStages], newPaths[bestPathOffset:bestPathOffset+int(psNLSF_CB.nStages)])

	referenceNLSFDecode(pNLSF_Q15, psNLSF_CB, NLSFIndices, LPC_order)
}

func TestBurgAndLPCMatchOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(923442))
	for _, order := range []int32{1, 2, 4, 8, 10, 12, 16} {
		for _, subframes := range []int32{1, 2, 4} {
			for _, subframeLength := range []int32{40, 56, 76, 96, 136} {
				for _, amplitude := range []int32{0, 4, 64, 1024, 32767} {
					input := make([]int16, subframes*subframeLength+8)
					for i := range input {
						if amplitude != 0 {
							input[i] = int16(rng.Int31n(amplitude*2+1) - amplitude)
						}
					}
					inputView := &slice[int16]{arr: input, offset: 3, size: 2}
					want, got := make([]int32, order+4), make([]int32, order+4)
					for i := range want {
						want[i] = -2938
						got[i] = -2938
					}
					var wantEnergy, wantQ, gotEnergy, gotQ int32
					referenceBurgModified(&wantEnergy, &wantQ, &slice[int32]{arr: want, offset: 2, size: 4}, inputView, subframeLength, subframes, FIX_CONST(FIND_LPC_COND_FAC, 32), order)
					burg_modified(&gotEnergy, &gotQ, &slice[int32]{arr: got, offset: 2, size: 4}, inputView, subframeLength, subframes, FIX_CONST(FIND_LPC_COND_FAC, 32), order)
					if gotEnergy != wantEnergy || gotQ != wantQ || !reflect.DeepEqual(got, want) {
						t.Fatalf("Burg order=%d subframes=%d length=%d amplitude=%d: energy (%d,%d) != (%d,%d)", order, subframes, subframeLength, amplitude, gotEnergy, gotQ, wantEnergy, wantQ)
					}
				}
			}
		}
	}
	for _, order := range []int32{10, 16} {
		for _, subframeLength := range []int32{40 + order, 80 + order, 120 + order} {
			for _, amplitude := range []int32{0, 4, 64, 1024, 32767} {
				for _, interpolate := range []int32{0, 1} {
					input := make([]int16, NB_SUBFR*subframeLength+8)
					for i := range input {
						if amplitude != 0 {
							input[i] = int16(rng.Int31n(amplitude*2+1) - amplitude)
						}
					}
					prev := make([]int32, order)
					for i := range prev {
						prev[i] = int32(i+1) * 32768 / (order + 1)
					}
					want, got := make([]int32, order+4), make([]int32, order+4)
					var wantIndex, gotIndex int32
					prevView := &slice[int32]{arr: prev, size: 4}
					inputView := &slice[int16]{arr: input, offset: 3, size: 2}
					referenceFindLPC(&slice[int32]{arr: want, offset: 2, size: 4}, &wantIndex, prevView, interpolate, order, inputView, subframeLength)
					find_LPC_FIX(&slice[int32]{arr: got, offset: 2, size: 4}, &gotIndex, prevView, interpolate, order, inputView, subframeLength)
					if gotIndex != wantIndex || !reflect.DeepEqual(got, want) {
						t.Fatalf("find LPC order=%d length=%d amplitude=%d interpolate=%d: %d != %d", order, subframeLength, amplitude, interpolate, gotIndex, wantIndex)
					}
				}
			}
		}
	}
}

func TestNLSFEncodeMatchOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(923443))
	for _, test := range []struct {
		codebook *NLSF_CB_struct
		order    int32
	}{
		{&NLSF_CB0_10, 10}, {&NLSF_CB1_10, 10}, {&NLSF_CB0_16, 16}, {&NLSF_CB1_16, 16},
	} {
		for _, survivors := range []int32{2, 4, 16} {
			for _, deactivate := range []int32{0, 1} {
				for trial := 0; trial < 20; trial++ {
					want, got := make([]int32, test.order+4), make([]int32, test.order+4)
					previous := make([]int32, test.order+4)
					weights := make([]int32, test.order+4)
					for i := int32(0); i < test.order; i++ {
						want[i+2] = (i+1)*32768/(test.order+1) + rng.Int31n(601) - 300
						previous[i+2] = (i+1)*32768/(test.order+1) + rng.Int31n(601) - 300
						weights[i+2] = rng.Int31n(2048) + 1
					}
					copy(got, want)
					wantIndices, gotIndices := make([]int32, test.codebook.nStages+4), make([]int32, test.codebook.nStages+4)
					prevView := &slice[int32]{arr: previous, offset: 2, size: 4}
					weightView := &slice[int32]{arr: weights, offset: 2, size: 4}
					mu := rng.Int31n(500) + 1
					reduction := rng.Int31n(16000)
					referenceNLSFEncode(&slice[int32]{arr: wantIndices, offset: 2, size: 4}, &slice[int32]{arr: want, offset: 2, size: 4}, test.codebook, prevView, weightView, mu, reduction, survivors, test.order, deactivate)
					NLSF_MSVQ_encode_FIX(&slice[int32]{arr: gotIndices, offset: 2, size: 4}, &slice[int32]{arr: got, offset: 2, size: 4}, test.codebook, prevView, weightView, mu, reduction, survivors, test.order, deactivate)
					if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotIndices, wantIndices) {
						t.Fatalf("NLSF codebook=%p order=%d survivors=%d deactivate=%d trial=%d", test.codebook, test.order, survivors, deactivate, trial)
					}
				}
			}
		}
	}
}

func BenchmarkPredictionFilters(b *testing.B) {
	input := make([]int16, 544)
	for i := range input {
		input[i] = int16(i * 7591)
	}
	in := &slice[int16]{arr: input, size: 2}
	for _, test := range []struct {
		name string
		run  func(*slice[int32], *int32, *slice[int16], int16, int32, int32)
	}{
		{"warped-original", referenceWarpedAutocorrelation}, {"warped-phase-two", referenceWarpedAutocorrelationPhaseTwo}, {"warped-optimized", warped_autocorrelation_FIX},
	} {
		b.Run(test.name, func(b *testing.B) {
			var corr [17]int32
			view := &slice[int32]{arr: corr[:], size: 4}
			var scale int32
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				test.run(view, &scale, in, 9830, 360, 16)
			}
		})
	}
	for _, test := range []struct {
		name string
		run  func(*slice[int16], *slice[int16], *slice[int16], *slice[int16], int32, int32)
	}{
		{"LPC-original", referenceLPCAnalysisFilter}, {"LPC-phase-two", referenceLPCAnalysisFilterPhaseTwo}, {"LPC-optimized", LPC_analysis_filter},
	} {
		b.Run(test.name, func(b *testing.B) {
			var coeff, state [16]int16
			var output [544]int16
			for i := range coeff {
				coeff[i] = int16(i * 983)
			}
			coefficients := &slice[int16]{arr: coeff[:], size: 2}
			stateView := &slice[int16]{arr: state[:], size: 2}
			out := &slice[int16]{arr: output[:], size: 2}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				test.run(in, coefficients, stateView, out, 544, 16)
			}
		})
	}
	for _, test := range []struct {
		name string
		run  func(*int32, *int32, *slice[int32], *slice[int16], int32, int32, int32, int32)
	}{
		{"burg-original", referenceBurgModified}, {"burg-optimized", burg_modified},
	} {
		b.Run(test.name, func(b *testing.B) {
			var coeff [16]int32
			var energy, scale int32
			coefficients := &slice[int32]{arr: coeff[:], size: 4}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				test.run(&energy, &scale, coefficients, in, 136, 4, FIX_CONST(FIND_LPC_COND_FAC, 32), 16)
			}
		})
	}
}

func referenceNLSFSumError(
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

func TestNLSFSumErrorMatchOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(923444))
	for _, order := range []int32{2, 4, 8, 10, 12, 16} {
		for _, vectors := range []int32{0, 1, 2, 4, 16} {
			for _, codewords := range []int32{0, 1, 8, 16, 64, 128} {
				for trial := 0; trial < 5; trial++ {
					input := make([]int32, vectors*order+4)
					weights := make([]int32, order+4)
					codebook := make([]int16, codewords*order+4)
					for i := range input {
						input[i] = int32(rng.Uint32())
					}
					for i := range weights {
						weights[i] = int32(rng.Uint32())
					}
					for i := range codebook {
						codebook[i] = int16(rng.Uint32())
					}
					want, got := make([]int32, vectors*codewords+4), make([]int32, vectors*codewords+4)
					for i := range want {
						want[i] = 2147418113
						got[i] = 2147418113
					}
					in := &slice[int32]{arr: input, offset: 2, size: 4}
					w := &slice[int32]{arr: weights, offset: 2, size: 4}
					cb := &slice[int16]{arr: codebook, offset: 2, size: 2}
					referenceNLSFSumError(&slice[int32]{arr: want, offset: 2, size: 4}, in, w, cb, vectors, codewords, order)
					NLSF_VQ_sum_error_FIX(&slice[int32]{arr: got, offset: 2, size: 4}, in, w, cb, vectors, codewords, order)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("NLSF errors order=%d vectors=%d codewords=%d trial=%d", order, vectors, codewords, trial)
					}
				}
			}
		}
	}
}

func BenchmarkNLSFSumError(b *testing.B) {
	for _, order := range []int32{10, 16} {
		for _, counts := range [][2]int32{{1, 128}, {16, 16}} {
			input := make([]int32, counts[0]*order)
			weights := make([]int32, order)
			codebook := make([]int16, counts[1]*order)
			errors := make([]int32, counts[0]*counts[1])
			for i := range input {
				input[i] = int32(i * 913)
			}
			for i := range weights {
				weights[i] = int32(i*31 + 64)
			}
			for i := range codebook {
				codebook[i] = int16(i * 823)
			}
			in := &slice[int32]{arr: input, size: 4}
			w := &slice[int32]{arr: weights, size: 4}
			cb := &slice[int16]{arr: codebook, size: 2}
			out := &slice[int32]{arr: errors, size: 4}
			for _, test := range []struct {
				name string
				run  func(*slice[int32], *slice[int32], *slice[int32], *slice[int16], int32, int32, int32)
			}{
				{"original", referenceNLSFSumError}, {"phase-two", referenceNLSFSumErrorPhaseTwo}, {"optimized", NLSF_VQ_sum_error_FIX},
			} {
				b.Run(fmt.Sprintf("order%d/N%dK%d/%s", order, counts[0], counts[1], test.name), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						test.run(out, in, w, cb, counts[0], counts[1], order)
					}
				})
			}
		}
	}
}

func referenceNLSFSumErrorPhaseTwo(
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

func referenceWarpedAutocorrelationPhaseTwo(corr *slice[int32], scale *int32, input *slice[int16], warping_Q16 int16, length, order int32) {
	var (
		n, i, lsh        int32
		tmp1_QS, tmp2_QS int32
		state_QS         [MAX_SHAPE_LPC_ORDER + 1]int32
		corr_QC          [MAX_SHAPE_LPC_ORDER + 1]int64
	)
	samples := input.From()
	correlations := corr.From()

	for n = 0; n < length; n++ {
		tmp1_QS = LSHIFT32(int32(samples[n]), QS)
		for i = 0; i < order; i += 2 {
			tmp2_QS = SMLAWB(state_QS[i], state_QS[i+1]-tmp1_QS, int32(warping_Q16))
			state_QS[i] = tmp1_QS
			corr_QC[i] += RSHIFT64(SMULL(tmp1_QS, state_QS[0]), 2*QS-QC)
			tmp1_QS = SMLAWB(state_QS[i+1], state_QS[i+2]-tmp2_QS, int32(warping_Q16))
			state_QS[i+1] = tmp2_QS
			corr_QC[i+1] += RSHIFT64(SMULL(tmp2_QS, state_QS[0]), 2*QS-QC)
		}
		state_QS[order] = tmp1_QS
		corr_QC[order] += RSHIFT64(SMULL(tmp1_QS, state_QS[0]), 2*QS-QC)
	}

	lsh = CLZ64(corr_QC[0]) - 35
	lsh = LIMIT(lsh, -12-QC, 30-QC)
	*scale = -(QC + lsh)

	if lsh >= 0 {
		for i = 0; i < order+1; i++ {
			correlations[i] = int32(LSHIFT64(corr_QC[i], lsh))
		}
	} else {
		for i = 0; i < order+1; i++ {
			correlations[i] = int32(RSHIFT64(corr_QC[i], -lsh))
		}
	}
}

func referenceLPCAnalysisFilterPhaseTwo(
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
