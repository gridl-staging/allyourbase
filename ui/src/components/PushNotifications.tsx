import { useCallback, useEffect, useState } from "react";
import type { PushDelivery, PushDeviceToken } from "../types";
import {
  adminSendPush,
  getAdminPushDelivery,
  listAdminPushDeliveries,
  listAdminPushDevices,
  registerAdminPushDevice,
  revokeAdminPushDevice,
} from "../api";
import { AlertCircle, Loader2 } from "lucide-react";
import { cn } from "../lib/utils";
import { useAppToast } from "./ToastProvider";
import { PushNotificationsDeliveriesTab } from "./push-notifications/PushNotificationsDeliveriesTab";
import { PushNotificationsDevicesTab } from "./push-notifications/PushNotificationsDevicesTab";
import { PushNotificationsRegisterModal } from "./push-notifications/PushNotificationsRegisterModal";
import { PushNotificationsSendModal } from "./push-notifications/PushNotificationsSendModal";
import {
  EMPTY_DELIVERY_FILTERS,
  EMPTY_DEVICE_FILTERS,
  EMPTY_REGISTER_FORM,
  EMPTY_SEND_FORM,
  type DeliveryFilters,
  type DeviceFilters,
  type PushNotificationsTab,
  type RegisterFormState,
  type SendFormState,
} from "./push-notifications/models";
import { parsePushDataJSON } from "./push-notifications/helpers";

export function PushNotifications() {
  const [tab, setTab] = useState<PushNotificationsTab>("devices");

  const [devices, setDevices] = useState<PushDeviceToken[] | null>(null);
  const [loadingDevices, setLoadingDevices] = useState(true);
  const [devicesError, setDevicesError] = useState<string | null>(null);

  const [deliveries, setDeliveries] = useState<PushDelivery[] | null>(null);
  const [loadingDeliveries, setLoadingDeliveries] = useState(false);
  const [deliveriesError, setDeliveriesError] = useState<string | null>(null);

  const [deviceFilters, setDeviceFilters] = useState<DeviceFilters>(EMPTY_DEVICE_FILTERS);
  const [appliedDeviceFilters, setAppliedDeviceFilters] = useState<DeviceFilters>(EMPTY_DEVICE_FILTERS);
  const [deliveryFilters, setDeliveryFilters] = useState<DeliveryFilters>(EMPTY_DELIVERY_FILTERS);
  const [appliedDeliveryFilters, setAppliedDeliveryFilters] = useState<DeliveryFilters>(EMPTY_DELIVERY_FILTERS);

  const [registerOpen, setRegisterOpen] = useState(false);
  const [registering, setRegistering] = useState(false);
  const [registerForm, setRegisterForm] = useState<RegisterFormState>(EMPTY_REGISTER_FORM);

  const [sendOpen, setSendOpen] = useState(false);
  const [sending, setSending] = useState(false);
  const [sendForm, setSendForm] = useState<SendFormState>(EMPTY_SEND_FORM);

  const [expandedDeliveryIDs, setExpandedDeliveryIDs] = useState<Set<string>>(new Set());
  const [deliveryDetails, setDeliveryDetails] = useState<Record<string, PushDelivery>>({});
  const [detailLoadingID, setDetailLoadingID] = useState<string | null>(null);

  const { addToast } = useAppToast();

  const loadDevices = useCallback(async (filters: DeviceFilters) => {
    setLoadingDevices(true);
    try {
      setDevicesError(null);
      const response = await listAdminPushDevices(filters);
      setDevices(response.items);
    } catch (error) {
      setDevicesError(error instanceof Error ? error.message : "Failed to load devices");
      setDevices(null);
    } finally {
      setLoadingDevices(false);
    }
  }, []);

  const loadDeliveries = useCallback(async (filters: DeliveryFilters) => {
    setLoadingDeliveries(true);
    try {
      setDeliveriesError(null);
      const response = await listAdminPushDeliveries(filters);
      setDeliveries(response.items);
    } catch (error) {
      setDeliveriesError(error instanceof Error ? error.message : "Failed to load deliveries");
      setDeliveries(null);
    } finally {
      setLoadingDeliveries(false);
    }
  }, []);

  useEffect(() => {
    void loadDevices(EMPTY_DEVICE_FILTERS);
  }, [loadDevices]);

  useEffect(() => {
    if (tab !== "deliveries" || deliveries !== null) {
      return;
    }
    void loadDeliveries(appliedDeliveryFilters);
  }, [tab, deliveries, loadDeliveries, appliedDeliveryFilters]);

  const handleApplyDeviceFilters = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nextFilters = {
      app_id: deviceFilters.app_id.trim(),
      user_id: deviceFilters.user_id.trim(),
      include_inactive: deviceFilters.include_inactive,
    };
    setAppliedDeviceFilters(nextFilters);
    setDeviceFilters(nextFilters);
    await loadDevices(nextFilters);
  };

  const handleApplyDeliveryFilters = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nextFilters = {
      app_id: deliveryFilters.app_id.trim(),
      user_id: deliveryFilters.user_id.trim(),
      status: deliveryFilters.status,
    };
    setAppliedDeliveryFilters(nextFilters);
    await loadDeliveries(nextFilters);
  };

  const handleRevokeDevice = async (id: string) => {
    try {
      await revokeAdminPushDevice(id);
      addToast("success", `Revoked device ${id}`);
      await loadDevices(appliedDeviceFilters);
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to revoke device");
    }
  };

  const handleRegisterSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const appID = registerForm.app_id.trim();
    const userID = registerForm.user_id.trim();
    const token = registerForm.token.trim();
    if (!appID || !userID || !token) {
      addToast("error", "App ID, User ID, and Token are required.");
      return;
    }

    setRegistering(true);
    try {
      await registerAdminPushDevice({
        app_id: appID,
        user_id: userID,
        provider: registerForm.provider,
        platform: registerForm.platform,
        token,
        device_name: registerForm.device_name.trim() || undefined,
      });
      addToast("success", "Device registered");
      setRegisterOpen(false);
      setRegisterForm(EMPTY_REGISTER_FORM);
      await loadDevices(appliedDeviceFilters);
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to register device");
    } finally {
      setRegistering(false);
    }
  };

  const handleToggleDeliveryDetail = async (id: string) => {
    if (expandedDeliveryIDs.has(id)) {
      const nextExpandedDeliveryIDs = new Set(expandedDeliveryIDs);
      nextExpandedDeliveryIDs.delete(id);
      setExpandedDeliveryIDs(nextExpandedDeliveryIDs);
      return;
    }

    if (!deliveryDetails[id]) {
      setDetailLoadingID(id);
      try {
        const detail = await getAdminPushDelivery(id);
        setDeliveryDetails((prev) => ({ ...prev, [id]: detail }));
      } catch (error) {
        addToast("error", error instanceof Error ? error.message : "Failed to load delivery detail");
        setDetailLoadingID(null);
        return;
      } finally {
        setDetailLoadingID(null);
      }
    }

    const nextExpandedDeliveryIDs = new Set(expandedDeliveryIDs);
    nextExpandedDeliveryIDs.add(id);
    setExpandedDeliveryIDs(nextExpandedDeliveryIDs);
  };

  const handleSendSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const appID = sendForm.app_id.trim();
    const userID = sendForm.user_id.trim();
    const title = sendForm.title.trim();
    const body = sendForm.body.trim();

    if (!appID || !userID || !title || !body) {
      addToast("error", "App ID, User ID, Title, and Body are required.");
      return;
    }

    const parsedData = parsePushDataJSON(sendForm.dataJSON);
    if (parsedData.error) {
      addToast("error", parsedData.error);
      return;
    }

    setSending(true);
    try {
      await adminSendPush({
        app_id: appID,
        user_id: userID,
        title,
        body,
        data: parsedData.data ?? {},
      });
      addToast("success", "Push delivery queued");
      setSendOpen(false);
      setSendForm(EMPTY_SEND_FORM);
      await loadDeliveries(appliedDeliveryFilters);
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to send push notification");
    } finally {
      setSending(false);
    }
  };

  if (loadingDevices && devices === null) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading push notifications...
      </div>
    );
  }

  if (devicesError && devices === null) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{devicesError}</p>
          <button
            onClick={() => {
              void loadDevices(appliedDeviceFilters);
            }}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Push Notifications</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          Manage mobile push device tokens and delivery audit history
        </p>
      </div>

      <div className="mb-4 flex items-center gap-2">
        <button
          onClick={() => setTab("devices")}
          className={cn(
            "px-3 py-1.5 text-sm rounded border",
            tab === "devices"
              ? "bg-gray-900 text-white border-gray-900"
              : "bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200",
          )}
        >
          Devices
        </button>
        <button
          onClick={() => setTab("deliveries")}
          className={cn(
            "px-3 py-1.5 text-sm rounded border",
            tab === "deliveries"
              ? "bg-gray-900 text-white border-gray-900"
              : "bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-200",
          )}
        >
          Deliveries
        </button>
      </div>

      {tab === "devices" ? (
        <PushNotificationsDevicesTab
          deviceFilters={deviceFilters}
          setDeviceFilters={setDeviceFilters}
          devices={devices}
          onApplyDeviceFilters={handleApplyDeviceFilters}
          onOpenRegister={() => setRegisterOpen(true)}
          onRevokeDevice={handleRevokeDevice}
        />
      ) : (
        <PushNotificationsDeliveriesTab
          deliveryFilters={deliveryFilters}
          setDeliveryFilters={setDeliveryFilters}
          deliveries={deliveries ?? []}
          loadingDeliveries={loadingDeliveries}
          deliveriesError={deliveriesError}
          detailLoadingID={detailLoadingID}
          expandedDeliveryIDs={expandedDeliveryIDs}
          deliveryDetails={deliveryDetails}
          onApplyDeliveryFilters={handleApplyDeliveryFilters}
          onOpenSend={() => setSendOpen(true)}
          onToggleDeliveryDetail={handleToggleDeliveryDetail}
        />
      )}

      <PushNotificationsRegisterModal
        open={registerOpen}
        registering={registering}
        registerForm={registerForm}
        setRegisterForm={setRegisterForm}
        onClose={() => setRegisterOpen(false)}
        onSubmit={handleRegisterSubmit}
      />

      <PushNotificationsSendModal
        open={sendOpen}
        sending={sending}
        sendForm={sendForm}
        setSendForm={setSendForm}
        onClose={() => setSendOpen(false)}
        onSubmit={handleSendSubmit}
      />
    </div>
  );
}
