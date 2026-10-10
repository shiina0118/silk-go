package silk

import "math"

const (
	referenceQA = 16
)

var referenceA_LIMIT = FIX_CONST(0.99975, referenceQA)

func referenceLPC_inverse_pred_gain_QA(invGain_Q30 *int32, A_QA *slice[*slice[int32]], order int32) int32 {
	var (
		k, n, headrm                               int32
		rc_Q31, rc_mult1_Q30, rc_mult2_Q16, tmp_QA int32
		Aold_QA, Anew_QA                           *slice[int32]
	)

	Anew_QA = A_QA.idx(int(order & 1))

	*invGain_Q30 = 1 << 30
	for k = order - 1; k > 0; k-- {

		if (Anew_QA.idx(int(k)) > referenceA_LIMIT) || (Anew_QA.idx(int(k)) < -referenceA_LIMIT) {
			return 1
		}

		rc_Q31 = -LSHIFT(Anew_QA.idx(int(k)), 31-referenceQA)

		rc_mult1_Q30 = (math.MaxInt32 >> 1) - SMMUL(rc_Q31, rc_Q31)

		rc_mult2_Q16 = INVERSE32_varQ(rc_mult1_Q30, 46)

		*invGain_Q30 = LSHIFT(SMMUL(*invGain_Q30, rc_mult1_Q30), 2)

		Aold_QA = Anew_QA
		Anew_QA = A_QA.idx(int(k & 1))

		headrm = CLZ32(rc_mult2_Q16) - 1
		rc_mult2_Q16 = LSHIFT(rc_mult2_Q16, headrm)

		for n = 0; n < k; n++ {
			tmp_QA = Aold_QA.idx(int(n)) - LSHIFT(SMMUL(Aold_QA.idx(int(k-n-1)), rc_Q31), 1)
			*Anew_QA.ptr(int(n)) = LSHIFT(SMMUL(tmp_QA, rc_mult2_Q16), 16-headrm)
		}
	}

	if (Anew_QA.idx(0) > referenceA_LIMIT) || (Anew_QA.idx(0) < -referenceA_LIMIT) {
		return 1
	}

	rc_Q31 = -LSHIFT(Anew_QA.idx(0), 31-referenceQA)

	rc_mult1_Q30 = (math.MaxInt32 >> 1) - SMMUL(rc_Q31, rc_Q31)

	*invGain_Q30 = LSHIFT(SMMUL(*invGain_Q30, rc_mult1_Q30), 2)

	return 0
}

func referenceLPC_inverse_pred_gain(invGain_Q30 *int32, A_Q12 *slice[int16], order int32) int32 {
	var (
		k       int32
		Atmp_QA = alloc[*slice[int32]](2)
		Anew_QA *slice[int32]
	)

	for i := 0; i < 2; i++ {
		*Atmp_QA.ptr(i) = alloc[int32](MAX_ORDER_LPC)
	}

	Anew_QA = Atmp_QA.idx(int(order & 1))

	for k = 0; k < order; k++ {
		*Anew_QA.ptr(int(k)) = LSHIFT(int32(A_Q12.idx(int(k))), referenceQA-12)
	}

	return referenceLPC_inverse_pred_gain_QA(invGain_Q30, Atmp_QA, order)
}

func referenceLPC_inverse_pred_gain_Q24(invGain_Q30 *int32, A_Q24 *slice[int32], order int32) int32 {
	var (
		k       int32
		Atmp_QA = alloc[*slice[int32]](2)
		Anew_QA *slice[int32]
	)

	for i := 0; i < 2; i++ {
		*Atmp_QA.ptr(i) = alloc[int32](MAX_ORDER_LPC)
	}

	Anew_QA = Atmp_QA.idx(int(order & 1))

	for k = 0; k < order; k++ {
		*Anew_QA.ptr(int(k)) = RSHIFT_ROUND(A_Q24.idx(int(k)), 24-referenceQA)
	}

	return referenceLPC_inverse_pred_gain_QA(invGain_Q30, Atmp_QA, order)
}

type referenceinv_D_t struct {
	Q36_part int32
	Q48_part int32
}

func referencesolve_LDL_FIX(A *slice[int32], M int32, b, x_Q16 *slice[int32]) {
	var (
		L_Q16 = alloc[int32](MAX_MATRIX_SIZE * MAX_MATRIX_SIZE)
		Y     = alloc[int32](MAX_MATRIX_SIZE)
		inv_D = [MAX_MATRIX_SIZE]referenceinv_D_t{}
	)

	referenceLDL_factorize_FIX(A, M, L_Q16, inv_D[:])

	referenceLS_SolveFirst_FIX(L_Q16, M, b, Y)

	referenceLS_divide_Q16_FIX(Y, inv_D[:], M)

	referenceLS_SolveLast_FIX(L_Q16, M, Y, x_Q16)
}

func referenceLDL_factorize_FIX(A *slice[int32], M int32, L_Q16 *slice[int32], inv_D []referenceinv_D_t) {
	var (
		i, j, k, status, loop_count                          int32
		ptr1, ptr2                                           *slice[int32]
		diag_min_value, tmp_32, err                          int32
		v_Q0                                                 = alloc[int32](MAX_MATRIX_SIZE)
		D_Q0                                                 = alloc[int32](MAX_MATRIX_SIZE)
		one_div_diag_Q36, one_div_diag_Q40, one_div_diag_Q48 int32
	)

	status = 1
	diag_min_value = max(SMMUL(ADD_SAT32(A.idx(0), A.idx(int(SMULBB(M, M)-1))), FIX_CONST(FIND_LTP_COND_FAC, 31)), 1<<9)
	for loop_count = 0; loop_count < M && status == 1; loop_count++ {
		status = 0
		for j = 0; j < M; j++ {
			ptr1 = matrix_adr(L_Q16, j, 0, M)
			tmp_32 = 0
			for i = 0; i < j; i++ {
				*v_Q0.ptr(int(i)) = SMULWW(D_Q0.idx(int(i)), ptr1.idx(int(i)))
				tmp_32 = SMLAWW(tmp_32, v_Q0.idx(int(i)), ptr1.idx(int(i)))
			}
			tmp_32 = SUB32(*matrix_ptr(A, j, j, M), tmp_32)

			if tmp_32 < diag_min_value {
				tmp_32 = SUB32(SMULBB(loop_count+1, diag_min_value), tmp_32)
				for i = 0; i < M; i++ {
					*matrix_ptr(A, i, i, M) = ADD32(*matrix_ptr(A, i, i, M), tmp_32)
				}
				status = 1
				break
			}
			*D_Q0.ptr(int(j)) = tmp_32

			one_div_diag_Q36 = INVERSE32_varQ(tmp_32, 36)
			one_div_diag_Q40 = LSHIFT(one_div_diag_Q36, 4)
			err = SUB32(1<<24, SMULWW(tmp_32, one_div_diag_Q40))
			one_div_diag_Q48 = SMULWW(err, one_div_diag_Q40)

			inv_D[j].Q36_part = one_div_diag_Q36
			inv_D[j].Q48_part = one_div_diag_Q48

			*matrix_ptr(L_Q16, j, j, M) = 65536
			ptr1 = matrix_adr(A, j, 0, M)
			ptr2 = matrix_adr(L_Q16, j+1, 0, M)
			for i = j + 1; i < M; i++ {
				tmp_32 = 0
				for k = 0; k < j; k++ {
					tmp_32 = SMLAWW(tmp_32, v_Q0.idx(int(k)), ptr2.idx(int(k)))
				}
				tmp_32 = SUB32(ptr1.idx(int(i)), tmp_32)

				*matrix_ptr(L_Q16, i, j, M) = ADD32(SMMUL(tmp_32, one_div_diag_Q48),
					RSHIFT(SMULWW(tmp_32, one_div_diag_Q36), 4))

				ptr2 = ptr2.off(int(M))
			}
		}
	}

}

func referenceLS_divide_Q16_FIX(T *slice[int32], inv_D []referenceinv_D_t, M int32) {
	var i, tmp_32, one_div_diag_Q36, one_div_diag_Q48 int32

	for i = 0; i < M; i++ {
		one_div_diag_Q36 = inv_D[i].Q36_part
		one_div_diag_Q48 = inv_D[i].Q48_part

		tmp_32 = T.idx(int(i))
		*T.ptr(int(i)) = ADD32(SMMUL(tmp_32, one_div_diag_Q48), RSHIFT(SMULWW(tmp_32, one_div_diag_Q36), 4))
	}
}

func referenceLS_SolveFirst_FIX(L_Q16 *slice[int32], M int32, b *slice[int32], x_Q16 *slice[int32]) {
	var (
		i, j   int32
		ptr32  *slice[int32]
		tmp_32 int32
	)

	for i = 0; i < M; i++ {
		ptr32 = matrix_adr(L_Q16, i, 0, M)
		tmp_32 = 0
		for j = 0; j < i; j++ {
			tmp_32 = SMLAWW(tmp_32, ptr32.idx(int(j)), x_Q16.idx(int(j)))
		}
		*x_Q16.ptr(int(i)) = SUB32(b.idx(int(i)), tmp_32)
	}
}

func referenceLS_SolveLast_FIX(L_Q16 *slice[int32], M int32, b *slice[int32], x_Q16 *slice[int32]) {
	var (
		i, j   int32
		ptr32  *slice[int32]
		tmp_32 int32
	)

	for i = M - 1; i >= 0; i-- {
		ptr32 = matrix_adr(L_Q16, 0, i, M)
		tmp_32 = 0
		for j = M - 1; j > i; j-- {
			tmp_32 = SMLAWW(tmp_32, ptr32.idx(int(SMULBB(j, M))), x_Q16.idx(int(j)))
		}
		*x_Q16.ptr(int(i)) = SUB32(b.idx(int(i)), tmp_32)
	}
}
