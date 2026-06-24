package serviceemail

// eligibleVerificationAccount ensures the verification flow is only sent to an
// active account whose current email still matches the normalized request email
// and has not already been verified.
func eligibleVerificationAccount(user *AccountSnapshot, email string) bool {
	if user == nil || user.ID == "" {
		return false
	}
	if normalizeAccountEmail(user.Email) != email {
		return false
	}
	if !user.Active {
		return false
	}
	return !user.EmailVerified
}

// verificationDelivery builds the email payload for the verification sender.
func verificationDelivery(token string, flow iamEmailFlowState) emailDelivery {
	return emailDelivery{
		To:       flow.Email,
		Subject:  "Email verification",
		Template: "iam/email/verification",
		Data: map[string]any{
			"token":      token,
			"user_id":    flow.UserID,
			"email":      flow.Email,
			"expires_at": flow.ExpiresAt,
		},
	}
}

// accountEmailVerified safely returns the email verification flag for an account snapshot.
func accountEmailVerified(user *AccountSnapshot) bool {
	return user != nil && user.EmailVerified
}
