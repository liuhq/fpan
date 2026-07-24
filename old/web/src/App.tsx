import { Route, Router } from "@solidjs/router"
import { lazy } from "solid-js"

const App = () => {
  return (
    <Router>
      <Route path="/" component={lazy(() => import("#/routes/Home.tsx"))} />
    </Router>
  )
}

export default App
