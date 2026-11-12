import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import QuotaWidget from "./QuotaWidget";
import { IsWidget } from "../wailsjs/go/main/Widget";
import "./style.css";

// The same bundle serves the main window and the floating quota widget.
void IsWidget()
  .catch(() => false)
  .then((isWidget) => {
    ReactDOM.createRoot(document.getElementById("root")!).render(
      <React.StrictMode>{isWidget ? <QuotaWidget /> : <App />}</React.StrictMode>,
    );
  });
