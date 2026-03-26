import type { PushDeliveryStatus } from "../../types";

export type PushNotificationsTab = "devices" | "deliveries";

export type DeliveryFilters = {
  app_id: string;
  user_id: string;
  status: "" | PushDeliveryStatus;
};

export type DeviceFilters = {
  app_id: string;
  user_id: string;
  include_inactive: boolean;
};

export interface RegisterFormState {
  app_id: string;
  user_id: string;
  provider: "fcm" | "apns";
  platform: "android" | "ios";
  token: string;
  device_name: string;
}

export interface SendFormState {
  app_id: string;
  user_id: string;
  title: string;
  body: string;
  dataJSON: string;
}

export const EMPTY_REGISTER_FORM: RegisterFormState = {
  app_id: "",
  user_id: "",
  provider: "fcm",
  platform: "android",
  token: "",
  device_name: "",
};

export const EMPTY_SEND_FORM: SendFormState = {
  app_id: "",
  user_id: "",
  title: "",
  body: "",
  dataJSON: "{}",
};

export const EMPTY_DEVICE_FILTERS: DeviceFilters = {
  app_id: "",
  user_id: "",
  include_inactive: false,
};

export const EMPTY_DELIVERY_FILTERS: DeliveryFilters = {
  app_id: "",
  user_id: "",
  status: "",
};

export const DELIVERY_STATUS_OPTIONS: Array<{ value: "" | PushDeliveryStatus; label: string }> = [
  { value: "", label: "All statuses" },
  { value: "pending", label: "pending" },
  { value: "sent", label: "sent" },
  { value: "failed", label: "failed" },
  { value: "invalid_token", label: "invalid_token" },
];
