import { Routes, Route } from 'react-router-dom'
import { Suspense, lazy } from 'react'
import AppShell from './components/AppShell'
import { Spinner } from './components/ui'

const Overview = lazy(() => import('./pages/Overview'))
const AmphoraPage = lazy(() => import('./pages/AmphoraPage'))
const ParamdoraPage = lazy(() => import('./pages/ParamdoraPage'))
const HephaestusPage = lazy(() => import('./pages/HephaestusPage'))
const OrpheusPage = lazy(() => import('./pages/OrpheusPage'))
const ClioPage = lazy(() => import('./pages/ClioPage'))
const MnemePage = lazy(() => import('./pages/MnemePage'))
const IrisPage = lazy(() => import('./pages/IrisPage'))
const ThemisPage = lazy(() => import('./pages/ThemisPage'))

function Loading() {
  return (
    <div style={{ padding: 48, textAlign: 'center' }}>
      <Spinner />
    </div>
  )
}

export default function App() {
  return (
    <Suspense fallback={<Loading />}>
      <Routes>
        <Route element={<AppShell />}>
          <Route index element={<Overview />} />
          <Route path="amphora" element={<AmphoraPage />} />
          <Route path="paramdora" element={<ParamdoraPage />} />
          <Route path="hephaestus" element={<HephaestusPage />} />
          <Route path="orpheus" element={<OrpheusPage />} />
          <Route path="clio" element={<ClioPage />} />
          <Route path="mneme" element={<MnemePage />} />
          <Route path="iris" element={<IrisPage />} />
          <Route path="themis" element={<ThemisPage />} />
          <Route path="*" element={<Overview />} />
        </Route>
      </Routes>
    </Suspense>
  )
}