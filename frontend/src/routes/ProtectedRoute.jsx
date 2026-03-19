import React, { useEffect, useState } from "react";
import { Navigate, Outlet } from "react-router";
import { useAuth } from "../Auth/provider/AuthProvider";

export const ProtectedRoute = () => {
  const { isLoggedIn } = useAuth() || {};
  const [loggedIn, setLoggedIn] = useState(null);

  useEffect(() => {
    if (!isLoggedIn) return;
    isLoggedIn()
      .then(setLoggedIn)
      .catch(() => setLoggedIn(false));
  }, [isLoggedIn]);

  useEffect(() => {
    if (loggedIn === false) {
      window.location.href = "/oidc/login";
    }
  }, [loggedIn]);

  // // Check if the user is authenticated
  // if (loggedIn === false) {
  //   window.location.href = "/oidc/login";
  //   // // If not authenticated, redirect to the login page
  //   // return <Navigate to="/oidc/login" target="_self" />;
  //   // return <Navigate to="/login" />;
  // }

  if (loggedIn === false) {
    return null;
  }

  if (loggedIn === null) {
    // Still loading
    return null;
  }

  // If authenticated, render the child routes
  return <Outlet />;
};
