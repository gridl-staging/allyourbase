export interface AuthHooksConfig {
  before_sign_up: string;
  after_sign_up: string;
  custom_access_token: string;
  before_password_reset: string;
  send_email: string;
  send_sms: string;
}
