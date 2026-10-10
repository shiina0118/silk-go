package silk

import (
	"math"
)

const (
	QA = 16
)

var A_LIMIT = FIX_CONST(0.99975, QA)

func LPC_inverse_pred_gain_QA(invGain_Q30 *int32, A_QA *slice[*slice[int32]], order int32) int32 {
	return lpcInversePredictionGain(invGain_Q30, [2][]int32{A_QA.idx(0).From(), A_QA.idx(1).From()}, order)
}

func lpcInversePredictionGain(invGain_Q30 *int32, coefficients [2][]int32, order int32) int32 {
	var (
		k, n, headrm                               int32
		rc_Q31, rc_mult1_Q30, rc_mult2_Q16, tmp_QA int32
		Aold_QA, Anew_QA                           []int32
	)

	Anew_QA = coefficients[order&1]

	*invGain_Q30 = 1 << 30
	for k = order - 1; k > 0; k-- {

		if (Anew_QA[k] > A_LIMIT) || (Anew_QA[k] < -A_LIMIT) {
			return 1
		}

		rc_Q31 = -LSHIFT(Anew_QA[k], 31-QA)

		rc_mult1_Q30 = (math.MaxInt32 >> 1) - SMMUL(rc_Q31, rc_Q31)

		rc_mult2_Q16 = INVERSE32_varQ(rc_mult1_Q30, 46)

		*invGain_Q30 = LSHIFT(SMMUL(*invGain_Q30, rc_mult1_Q30), 2)

		Aold_QA = Anew_QA
		Anew_QA = coefficients[k&1]

		headrm = CLZ32(rc_mult2_Q16) - 1
		rc_mult2_Q16 = LSHIFT(rc_mult2_Q16, headrm)

		for n = 0; n < k; n++ {
			tmp_QA = Aold_QA[n] - LSHIFT(SMMUL(Aold_QA[k-n-1], rc_Q31), 1)
			Anew_QA[n] = LSHIFT(SMMUL(tmp_QA, rc_mult2_Q16), 16-headrm)
		}
	}

	if (Anew_QA[0] > A_LIMIT) || (Anew_QA[0] < -A_LIMIT) {
		return 1
	}

	rc_Q31 = -LSHIFT(Anew_QA[0], 31-QA)

	rc_mult1_Q30 = (math.MaxInt32 >> 1) - SMMUL(rc_Q31, rc_Q31)

	*invGain_Q30 = LSHIFT(SMMUL(*invGain_Q30, rc_mult1_Q30), 2)

	return 0
}

func LPC_inverse_pred_gain(invGain_Q30 *int32, A_Q12 *slice[int16], order int32) int32 {
	var coefficients [2][MAX_ORDER_LPC]int32
	current := coefficients[order&1][:]
	input := A_Q12.From()
	for k := int32(0); k < order; k++ {
		current[k] = LSHIFT(int32(input[k]), QA-12)
	}
	return lpcInversePredictionGain(invGain_Q30, [2][]int32{coefficients[0][:], coefficients[1][:]}, order)
}

func LPC_inverse_pred_gain_Q24(invGain_Q30 *int32, A_Q24 *slice[int32], order int32) int32 {
	var coefficients [2][MAX_ORDER_LPC]int32
	current := coefficients[order&1][:]
	input := A_Q24.From()
	for k := int32(0); k < order; k++ {
		current[k] = RSHIFT_ROUND(input[k], 24-QA)
	}
	return lpcInversePredictionGain(invGain_Q30, [2][]int32{coefficients[0][:], coefficients[1][:]}, order)
}
