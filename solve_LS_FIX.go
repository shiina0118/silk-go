package silk

type inv_D_t struct {
	Q36_part int32
	Q48_part int32
}

func solve_LDL_FIX(A *slice[int32], M int32, b, x_Q16 *slice[int32]) {
	var (
		matrix [MAX_MATRIX_SIZE * MAX_MATRIX_SIZE]int32
		vector [MAX_MATRIX_SIZE]int32
		inv_D  = [MAX_MATRIX_SIZE]inv_D_t{}
	)
	L_Q16 := mem2Slice(matrix[:])
	Y := mem2Slice(vector[:])

	LDL_factorize_FIX(A, M, L_Q16, inv_D[:])

	LS_SolveFirst_FIX(L_Q16, M, b, Y)

	LS_divide_Q16_FIX(Y, inv_D[:], M)

	LS_SolveLast_FIX(L_Q16, M, Y, x_Q16)
}

func LDL_factorize_FIX(A *slice[int32], M int32, L_Q16 *slice[int32], inv_D []inv_D_t) {
	var (
		i, j, k, status, loop_count                          int32
		diag_min_value, tmp_32, err                          int32
		v_Q0                                                 [MAX_MATRIX_SIZE]int32
		D_Q0                                                 [MAX_MATRIX_SIZE]int32
		one_div_diag_Q36, one_div_diag_Q40, one_div_diag_Q48 int32
	)
	a, lower := A.From(), L_Q16.From()

	status = 1
	diag_min_value = max(SMMUL(ADD_SAT32(a[0], a[SMULBB(M, M)-1]), FIX_CONST(FIND_LTP_COND_FAC, 31)), 1<<9)
	for loop_count = 0; loop_count < M && status == 1; loop_count++ {
		status = 0
		for j = 0; j < M; j++ {
			row := lower[j*M:]
			tmp_32 = 0
			for i = 0; i < j; i++ {
				v_Q0[i] = SMULWW(D_Q0[i], row[i])
				tmp_32 = SMLAWW(tmp_32, v_Q0[i], row[i])
			}
			tmp_32 = SUB32(a[j*M+j], tmp_32)

			if tmp_32 < diag_min_value {
				tmp_32 = SUB32(SMULBB(loop_count+1, diag_min_value), tmp_32)
				for i = 0; i < M; i++ {
					a[i*M+i] = ADD32(a[i*M+i], tmp_32)
				}
				status = 1
				break
			}
			D_Q0[j] = tmp_32

			one_div_diag_Q36 = INVERSE32_varQ(tmp_32, 36)
			one_div_diag_Q40 = LSHIFT(one_div_diag_Q36, 4)
			err = SUB32(1<<24, SMULWW(tmp_32, one_div_diag_Q40))
			one_div_diag_Q48 = SMULWW(err, one_div_diag_Q40)

			inv_D[j].Q36_part = one_div_diag_Q36
			inv_D[j].Q48_part = one_div_diag_Q48

			lower[j*M+j] = 65536
			inputRow := a[j*M:]
			for i = j + 1; i < M; i++ {
				row := lower[i*M:]
				tmp_32 = 0
				for k = 0; k < j; k++ {
					tmp_32 = SMLAWW(tmp_32, v_Q0[k], row[k])
				}
				tmp_32 = SUB32(inputRow[i], tmp_32)

				row[j] = ADD32(SMMUL(tmp_32, one_div_diag_Q48),
					RSHIFT(SMULWW(tmp_32, one_div_diag_Q36), 4))

			}
		}
	}

}

func LS_divide_Q16_FIX(T *slice[int32], inv_D []inv_D_t, M int32) {
	var i, tmp_32, one_div_diag_Q36, one_div_diag_Q48 int32
	values := T.From()

	for i = 0; i < M; i++ {
		one_div_diag_Q36 = inv_D[i].Q36_part
		one_div_diag_Q48 = inv_D[i].Q48_part

		tmp_32 = values[i]
		values[i] = ADD32(SMMUL(tmp_32, one_div_diag_Q48), RSHIFT(SMULWW(tmp_32, one_div_diag_Q36), 4))
	}
}

func LS_SolveFirst_FIX(L_Q16 *slice[int32], M int32, b *slice[int32], x_Q16 *slice[int32]) {
	var (
		i, j   int32
		tmp_32 int32
	)
	lower, input, output := L_Q16.From(), b.From(), x_Q16.From()

	for i = 0; i < M; i++ {
		row := lower[i*M:]
		tmp_32 = 0
		for j = 0; j < i; j++ {
			tmp_32 = SMLAWW(tmp_32, row[j], output[j])
		}
		output[i] = SUB32(input[i], tmp_32)
	}
}

func LS_SolveLast_FIX(L_Q16 *slice[int32], M int32, b *slice[int32], x_Q16 *slice[int32]) {
	var (
		i, j   int32
		tmp_32 int32
	)
	lower, input, output := L_Q16.From(), b.From(), x_Q16.From()

	for i = M - 1; i >= 0; i-- {
		tmp_32 = 0
		for j = M - 1; j > i; j-- {
			tmp_32 = SMLAWW(tmp_32, lower[j*M+i], output[j])
		}
		output[i] = SUB32(input[i], tmp_32)
	}
}
