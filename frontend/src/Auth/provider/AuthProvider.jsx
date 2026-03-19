import React, {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { refreshJwtToken } from "../hooks/Api";
import PropTypes from "prop-types";
import { useConfig } from "../../provider/ConfigProvider";

const AuthContext = createContext();

const AuthProvider = ({ serverState, children }) => {
  const apiHost = useConfig().apiHost;
  const refreshingPromise = useRef(null);
  const [expiryDate, setExpiryDate] = useState(serverState.expiryDate);
  const userData = serverState.userData;

  useEffect(() => {
    if (!expiryDate) {
      if (refreshingPromise.current) {
        clearTimeout(refreshingPromise.current);
        refreshingPromise.current = null;
      }
      return;
    }

    if (refreshingPromise.current) {
      return;
    }
    const d = new Date(expiryDate);
    const now = new Date();
    const rand = Math.random() * 10000; // add random time to prevent multiple clients refreshing at the same time
  
    const timeout = Math.max(d.getTime() - now.getTime() - 30000 + rand, 0); // refresh 30 seconds before expiry

    refreshingPromise.current = setTimeout(() => {
        refreshJwtToken(apiHost)
          .then((response) => {
            setExpiryDate(response.expiryDate);
          })
          .catch((error) => {
            console.error("Critical error during token refresh:", error);
          })
          .finally(() => {
            refreshingPromise.current = null;
          });
      }, timeout);
  }, [expiryDate]);

  const isLoggedIn = () => {
    console.log(userData)
    return userData !== null;
  };

  const contextValue = useMemo(
    () => ({
      isLoggedIn: async () => isLoggedIn(),
      gravatarUrl: userData?.gravatarUrl ?? "",
      role: userData?.role ?? "user",
      username: userData?.username ?? "unknown",
      id: userData?.id ?? 0,
    }),
    [userData],
  );

  // Provide the authentication context to the children components
  return (
    <AuthContext.Provider value={contextValue}>{children}</AuthContext.Provider>
  );
};

export const useAuth = () => {
  return useContext(AuthContext);
};

AuthProvider.propTypes = {
  children: PropTypes.node.isRequired,
};

export default AuthProvider;
