import React from "react";
import "./App.css";
import AuthProvider from "./Auth/provider/AuthProvider";
import Routes from "./routes";
import SentryInitializer from "./components/SentryInitalizer";
import ConfigProvider from "./provider/ConfigProvider";

function App() {
  return (
    <ConfigProvider>
      <AuthProvider serverState={serverState}>
        <SentryInitializer>
          <Routes />
        </SentryInitializer>
      </AuthProvider>
    </ConfigProvider>
  );
}

export default App;
